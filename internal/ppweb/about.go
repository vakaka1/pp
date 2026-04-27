package ppweb

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	gitHubRepoSlug       = "vakaka1/pp"
	gitHubRepoURL        = "https://github.com/" + gitHubRepoSlug
	gitHubReleasesURL    = gitHubRepoURL + "/releases"
	gitHubIssuesURL      = gitHubRepoURL + "/issues"
	gitHubLatestRelease  = "https://api.github.com/repos/" + gitHubRepoSlug + "/releases/latest"
	ppWebUpdateUnit      = "pp-web-update"
	releaseCacheLifetime = 15 * time.Minute
)

var releaseVersionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

type gitHubRelease struct {
	TagName     string               `json:"tag_name"`
	Name        string               `json:"name"`
	HTMLURL     string               `json:"html_url"`
	Body        string               `json:"body"`
	PublishedAt time.Time            `json:"published_at"`
	Assets      []gitHubReleaseAsset `json:"assets"`
}

type gitHubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type releaseCacheEntry struct {
	Release   *gitHubRelease
	CheckedAt time.Time
	Error     string
}

type aboutPayload struct {
	App     aboutAppInfo     `json:"app"`
	GitHub  aboutGitHubInfo  `json:"github"`
	Release aboutReleaseInfo `json:"release"`
	Update  aboutUpdateInfo  `json:"update"`
}

type aboutAppInfo struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Version      string `json:"version"`
	BuildDate    string `json:"buildDate"`
	GitCommit    string `json:"gitCommit"`
	BinaryPath   string `json:"binaryPath"`
	FrontendDist string `json:"frontendDist"`
}

type aboutGitHubInfo struct {
	Repository  string `json:"repository"`
	URL         string `json:"url"`
	ReleasesURL string `json:"releasesUrl"`
	IssuesURL   string `json:"issuesUrl"`
}

type aboutReleaseInfo struct {
	CheckedAt         time.Time `json:"checkedAt"`
	CurrentVersion    string    `json:"currentVersion"`
	LatestVersion     string    `json:"latestVersion"`
	LatestName        string    `json:"latestName"`
	LatestURL         string    `json:"latestUrl"`
	LatestBody        string    `json:"latestBody"`
	LatestPublishedAt time.Time `json:"latestPublishedAt"`
	UpdateAvailable   bool      `json:"updateAvailable"`
	Severity          string    `json:"severity"`
	IndicatorTone     string    `json:"indicatorTone"`
	StatusLabel       string    `json:"statusLabel"`
	Error             string    `json:"error,omitempty"`
}

type aboutUpdateInfo struct {
	CanStart bool            `json:"canStart"`
	Mode     string          `json:"mode"`
	Status   updateRunStatus `json:"status"`
}

type updateRunStatus struct {
	State         string     `json:"state"`
	Message       string     `json:"message"`
	TargetVersion string     `json:"targetVersion,omitempty"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
}

type versionTriplet struct {
	Major int
	Minor int
	Patch int
}

func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request, _ *Admin) {
	payload, err := s.buildAboutPayload(r.Context(), isTruthy(r.URL.Query().Get("refresh")))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleAboutUpdate(w http.ResponseWriter, r *http.Request, _ *Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	status := s.readUpdateStatus()
	if status.State == "queued" || status.State == "running" {
		writeError(w, http.StatusConflict, "обновление уже запущено")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	release, _, releaseErr := s.fetchLatestRelease(ctx, true)
	if release == nil {
		if releaseErr == "" {
			releaseErr = "не удалось определить последний релиз"
		}
		writeError(w, http.StatusBadGateway, releaseErr)
		return
	}

	targetVersion := strings.TrimSpace(release.TagName)
	if targetVersion == "" {
		targetVersion = "latest"
	}

	releaseInfo := describeReleaseStatus(strings.TrimSpace(s.opts.Build.Version), release, time.Now().UTC(), releaseErr)
	if !releaseInfo.UpdateAvailable {
		message := releaseInfo.StatusLabel
		if strings.TrimSpace(message) == "" {
			message = "обновление не требуется"
		}
		writeError(w, http.StatusConflict, message)
		return
	}

	startedAt := time.Now().UTC()
	if err := s.writeUpdateStatus(updateRunStatus{
		State:         "queued",
		Message:       fmt.Sprintf("Обновление %s поставлено в очередь.", humanVersion(targetVersion)),
		TargetVersion: targetVersion,
		StartedAt:     timePtr(startedAt),
	}); err != nil {
		s.log.Warn("failed to pre-write update status", zap.Error(err))
	}

	mode, err := s.startReleaseUpdate(targetVersion)
	if err != nil {
		finishedAt := time.Now().UTC()
		_ = s.writeUpdateStatus(updateRunStatus{
			State:         "error",
			Message:       err.Error(),
			TargetVersion: targetVersion,
			StartedAt:     timePtr(startedAt),
			FinishedAt:    timePtr(finishedAt),
		})
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"mode":          mode,
		"targetVersion": targetVersion,
		"message":       fmt.Sprintf("Обновление %s запущено.", humanVersion(targetVersion)),
	})
}

func (s *Server) buildAboutPayload(ctx context.Context, forceRefresh bool) (*aboutPayload, error) {
	settings, err := s.store.GetAppSettings(ctx, s.opts.CoreConfigPath)
	if err != nil {
		return nil, err
	}

	releaseCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	latestRelease, checkedAt, releaseErr := s.fetchLatestRelease(releaseCtx, forceRefresh)
	currentVersion := strings.TrimSpace(s.opts.Build.Version)
	releaseInfo := describeReleaseStatus(currentVersion, latestRelease, checkedAt, releaseErr)

	executablePath, _ := os.Executable()
	updateMode := s.detectUpdateMode()

	return &aboutPayload{
		App: aboutAppInfo{
			Name:         settings.AppName,
			Description:  "PP Web управляет подключениями, ядром PP и выпуском клиентских конфигов через веб-интерфейс.",
			Version:      humanVersion(currentVersion),
			BuildDate:    s.opts.Build.BuildDate,
			GitCommit:    shortCommit(s.opts.Build.GitCommit),
			BinaryPath:   executablePath,
			FrontendDist: s.opts.FrontendDist,
		},
		GitHub: aboutGitHubInfo{
			Repository:  gitHubRepoSlug,
			URL:         gitHubRepoURL,
			ReleasesURL: gitHubReleasesURL,
			IssuesURL:   gitHubIssuesURL,
		},
		Release: releaseInfo,
		Update: aboutUpdateInfo{
			CanStart: updateMode != "disabled" && releaseInfo.UpdateAvailable && releaseInfo.LatestVersion != "",
			Mode:     updateMode,
			Status:   s.readUpdateStatus(),
		},
	}, nil
}

func (s *Server) detectUpdateMode() string {
	if s.serviceUnitExists(ppWebUpdateUnit) {
		return "service"
	}

	if s.serviceUnitExists("pp-web") && execPathAvailable("systemd-run") && os.Geteuid() == 0 {
		return "transient"
	}

	if s.canWriteReleaseTargets() {
		return "direct"
	}

	return "disabled"
}

func (s *Server) canWriteReleaseTargets() bool {
	executablePath, err := os.Executable()
	if err != nil {
		return false
	}

	ppPath, err := s.resolvePPCoreBinaryPath()
	if err != nil {
		return false
	}

	return pathLikelyWritable(executablePath) &&
		pathLikelyWritable(ppPath) &&
		pathLikelyWritable(s.opts.FrontendDist)
}

func pathLikelyWritable(path string) bool {
	if path == "" {
		return false
	}

	target := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		target = filepath.Dir(path)
	} else if err != nil {
		target = filepath.Dir(path)
	}

	testFile, err := os.CreateTemp(target, ".pp-web-write-check-*")
	if err != nil {
		return false
	}
	name := testFile.Name()
	_ = testFile.Close()
	_ = os.Remove(name)
	return true
}

func (s *Server) fetchLatestRelease(ctx context.Context, forceRefresh bool) (*gitHubRelease, time.Time, string) {
	s.releaseMu.Lock()
	cached := s.releaseCache
	s.releaseMu.Unlock()

	if !forceRefresh && cached.Release != nil && time.Since(cached.CheckedAt) < releaseCacheLifetime {
		return cached.Release, cached.CheckedAt, cached.Error
	}

	release, err := fetchGitHubRelease(ctx, gitHubLatestRelease)
	checkedAt := time.Now().UTC()
	next := releaseCacheEntry{
		Release:   release,
		CheckedAt: checkedAt,
	}

	if err != nil {
		next.Error = err.Error()
		if cached.Release != nil {
			next.Release = cached.Release
			next.CheckedAt = cached.CheckedAt
		}
	}

	s.releaseMu.Lock()
	s.releaseCache = next
	s.releaseMu.Unlock()

	return next.Release, next.CheckedAt, next.Error
}

func fetchGitHubRelease(ctx context.Context, endpoint string) (*gitHubRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "pp-web-release-checker")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("github release request failed: %s", message)
	}

	var release gitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to decode github release: %w", err)
	}
	return &release, nil
}

func describeReleaseStatus(currentVersion string, latest *gitHubRelease, checkedAt time.Time, fetchErr string) aboutReleaseInfo {
	info := aboutReleaseInfo{
		CheckedAt:      checkedAt,
		CurrentVersion: humanVersion(currentVersion),
		Severity:       "none",
		IndicatorTone:  "neutral",
		StatusLabel:    "Проверка обновлений недоступна.",
		Error:          fetchErr,
	}

	if latest == nil {
		if fetchErr == "" {
			info.StatusLabel = "Данные о релизе пока недоступны."
		}
		return info
	}

	info.LatestVersion = humanVersion(latest.TagName)
	info.LatestName = strings.TrimSpace(latest.Name)
	info.LatestURL = strings.TrimSpace(latest.HTMLURL)
	info.LatestBody = strings.TrimSpace(latest.Body)
	info.LatestPublishedAt = latest.PublishedAt

	currentTag := normalizeVersionTag(currentVersion)
	latestTag := normalizeVersionTag(latest.TagName)
	if currentTag == latestTag {
		info.StatusLabel = "Установлена актуальная версия."
		return info
	}
	if !isReleaseVersion(currentVersion) {
		info.StatusLabel = "Запущена локальная сборка. Автоматическое обновление из релиза недоступно."
		return info
	}
	if !isReleaseVersion(latest.TagName) {
		info.StatusLabel = fmt.Sprintf("Последний релиз %s использует нестандартный тег.", humanVersion(latest.TagName))
		return info
	}

	currentParts, currentOK := parseVersionTriplet(currentVersion)
	latestParts, latestOK := parseVersionTriplet(latest.TagName)

	switch {
	case currentOK && latestOK:
		switch compareVersions(latestParts, currentParts) {
		case 1:
			info.UpdateAvailable = true
			if latestParts.Major != currentParts.Major || latestParts.Minor != currentParts.Minor {
				info.Severity = "major"
				info.IndicatorTone = "danger"
				info.StatusLabel = fmt.Sprintf("Доступно крупное обновление %s.", humanVersion(latest.TagName))
			} else {
				info.Severity = "patch"
				info.IndicatorTone = "warning"
				info.StatusLabel = fmt.Sprintf("Доступно обновление %s.", humanVersion(latest.TagName))
			}
		case 0:
			info.StatusLabel = "Установлена актуальная версия."
		default:
			info.StatusLabel = "Установлена версия новее последнего релиза."
		}
	default:
		info.StatusLabel = "Не удалось сравнить текущую версию с последним релизом."
	}

	return info
}

func parseVersionTriplet(raw string) (versionTriplet, bool) {
	matches := releaseVersionPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(matches) != 4 {
		return versionTriplet{}, false
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return versionTriplet{}, false
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return versionTriplet{}, false
	}
	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return versionTriplet{}, false
	}

	return versionTriplet{Major: major, Minor: minor, Patch: patch}, true
}

func compareVersions(left, right versionTriplet) int {
	switch {
	case left.Major != right.Major:
		return compareInts(left.Major, right.Major)
	case left.Minor != right.Minor:
		return compareInts(left.Minor, right.Minor)
	default:
		return compareInts(left.Patch, right.Patch)
	}
}

func compareInts(left, right int) int {
	switch {
	case left > right:
		return 1
	case left < right:
		return -1
	default:
		return 0
	}
}

func humanVersion(raw string) string {
	version := strings.TrimSpace(raw)
	if version == "" {
		return "Неизвестно"
	}
	return strings.TrimPrefix(version, "v")
}

func normalizeVersionTag(raw string) string {
	return strings.TrimPrefix(strings.TrimSpace(raw), "v")
}

func isReleaseVersion(raw string) bool {
	_, ok := parseVersionTriplet(raw)
	return ok
}

func timePtr(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}

func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	switch {
	case commit == "" || commit == "none":
		return "—"
	case len(commit) > 7:
		return commit[:7]
	default:
		return commit
	}
}

func isTruthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func execPathAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (s *Server) startReleaseUpdate(targetVersion string) (string, error) {
	switch {
	case s.serviceUnitExists(ppWebUpdateUnit):
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		out, err := runPrivilegedCommand(ctx, "systemctl", "--no-block", "start", ppWebUpdateUnit)
		if err != nil {
			return "", fmt.Errorf("не удалось запустить службу обновления: %s", explainPrivilegedCommandFailure("systemctl start pp-web-update", out, err))
		}
		return "service", nil
	case s.serviceUnitExists("pp-web") && execPathAvailable("systemd-run") && os.Geteuid() == 0:
		if err := s.startTransientReleaseUpdate(targetVersion); err != nil {
			return "", err
		}
		return "transient", nil
	default:
		if err := s.startDirectReleaseUpdate(targetVersion); err != nil {
			return "", err
		}
		return "direct", nil
	}
}

func (s *Server) startTransientReleaseUpdate(targetVersion string) error {
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("не удалось определить путь к pp-web: %w", err)
	}

	ppPath, err := s.resolvePPCoreBinaryPath()
	if err != nil {
		return err
	}

	unitName := fmt.Sprintf("pp-web-self-update-%d", time.Now().UTC().Unix())
	args := []string{
		"--unit", unitName,
		"--collect",
		"--quiet",
		executablePath,
		"apply-release",
		"--repo", gitHubRepoSlug,
		"--tag", targetVersion,
		"--pp-path", ppPath,
		"--pp-web-path", executablePath,
		"--frontend-dist", s.opts.FrontendDist,
		"--status-path", s.updateStatusPath(),
		"--pp-service", "pp-core",
		"--web-service", "pp-web",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "systemd-run", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("не удалось создать transient update unit: %s", strings.TrimSpace(string(out)))
	}

	return nil
}

func (s *Server) startDirectReleaseUpdate(targetVersion string) error {
	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("не удалось определить путь к pp-web: %w", err)
	}

	ppPath, err := s.resolvePPCoreBinaryPath()
	if err != nil {
		return err
	}

	args := []string{
		"apply-release",
		"--repo", gitHubRepoSlug,
		"--tag", targetVersion,
		"--pp-path", ppPath,
		"--pp-web-path", executablePath,
		"--frontend-dist", s.opts.FrontendDist,
		"--status-path", s.updateStatusPath(),
		"--pp-service", "pp-core",
	}

	if !s.serviceUnitExists("pp-web") {
		args = append(args, "--web-service", "")
	}

	cmd := exec.Command(executablePath, args...)
	if s.opts.ProjectRoot != "" {
		cmd.Dir = s.opts.ProjectRoot
	}

	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	cmd.Stderr = &buffer

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("не удалось запустить updater: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		message := strings.TrimSpace(buffer.String())
		if message == "" && err != nil {
			message = err.Error()
		}
		if message == "" {
			message = "updater завершился сразу после запуска"
		}
		return errors.New(message)
	case <-time.After(400 * time.Millisecond):
		go func() {
			if err := <-done; err != nil {
				s.log.Warn("direct updater exited", zap.Error(err), zap.String("output", strings.TrimSpace(buffer.String())))
			}
		}()
		return nil
	}
}

func (s *Server) resolvePPCoreBinaryPath() (string, error) {
	status := s.inspectPPCoreBinary()
	if path, ok := status["path"].(string); ok && strings.TrimSpace(path) != "" {
		return path, nil
	}

	executablePath, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(executablePath), "pp")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}

	if _, err := os.Stat("/usr/local/bin/pp"); err == nil {
		return "/usr/local/bin/pp", nil
	}

	return "", errors.New("не удалось определить путь к бинарнику pp")
}

func (s *Server) updateStatusPath() string {
	return filepath.Join(filepath.Dir(s.opts.DatabasePath), "update-status.json")
}

func (s *Server) readUpdateStatus() updateRunStatus {
	payload, err := os.ReadFile(s.updateStatusPath())
	if err != nil {
		if !os.IsNotExist(err) {
			s.log.Warn("failed to read update status", zap.Error(err))
		}
		return updateRunStatus{
			State:   "idle",
			Message: "Обновление ещё не запускалось.",
		}
	}

	var status updateRunStatus
	if err := json.Unmarshal(payload, &status); err != nil {
		s.log.Warn("failed to decode update status", zap.Error(err))
		return updateRunStatus{
			State:   "error",
			Message: "Не удалось прочитать состояние обновления.",
		}
	}

	if strings.TrimSpace(status.State) == "" {
		status.State = "idle"
	}
	if strings.TrimSpace(status.Message) == "" {
		status.Message = "Состояние обновления не указано."
	}
	return status
}

func (s *Server) writeUpdateStatus(status updateRunStatus) error {
	if err := os.MkdirAll(filepath.Dir(s.updateStatusPath()), 0o755); err != nil {
		return err
	}
	return writeJSONFileAtomic(s.updateStatusPath(), status, 0o644)
}

func writeJSONFileAtomic(targetPath string, payload any, mode os.FileMode) error {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmpFile, err := os.CreateTemp(filepath.Dir(targetPath), filepath.Base(targetPath)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}

	if _, err := tmpFile.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmpFile.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func gitHubReleaseByTagURL(repo, tag string) string {
	return fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, url.PathEscape(tag))
}
