package ppweb

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type releaseApplyRequest struct {
	Repo         string
	Tag          string
	PPPath       string
	PPWebPath    string
	FrontendDist string
	StatusPath   string
	PPService    string
	WebService   string
	Rollback     bool
}

func RunReleaseApplyCommand(args []string) error {
	fs := flag.NewFlagSet("apply-release", flag.ContinueOnError)

	var request releaseApplyRequest
	fs.StringVar(&request.Repo, "repo", gitHubRepoSlug, "GitHub repository in owner/name form")
	fs.StringVar(&request.Tag, "tag", "latest", "Release tag to install")
	fs.StringVar(&request.PPPath, "pp-path", "", "Destination path for pp binary")
	fs.StringVar(&request.PPWebPath, "pp-web-path", "", "Destination path for pp-web binary")
	fs.StringVar(&request.FrontendDist, "frontend-dist", "", "Destination directory for pp-web frontend")
	fs.StringVar(&request.StatusPath, "status-path", "", "Status file path")
	fs.StringVar(&request.PPService, "pp-service", "pp-core", "Systemd unit to restart after installing pp")
	fs.StringVar(&request.WebService, "web-service", "pp-web", "Systemd unit to restart after installing pp-web")
	fs.BoolVar(&request.Rollback, "rollback", false, "Rollback to previous version from .bak files")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if request.Rollback {
		return applyReleaseRollback(request)
	}

	return applyReleaseUpdate(request)
}

func applyReleaseRollback(request releaseApplyRequest) error {
	now := time.Now().UTC()
	if err := writeCLIUpdateStatus(request.StatusPath, updateRunStatus{
		State:     "running",
		Message:   "Выполняется откат к предыдущей версии.",
		StartedAt: timePtr(now),
	}); err != nil {
		return err
	}

	if request.PPPath == "" || request.PPWebPath == "" || request.FrontendDist == "" {
		return finalizeCLIUpdateError(request.StatusPath, "rollback", now, fmt.Errorf("пути для отката заданы не полностью"))
	}

	// Откат бинарников
	if err := rollbackFile(request.PPPath); err != nil {
		return finalizeCLIUpdateError(request.StatusPath, "rollback", now, err)
	}
	if err := rollbackFile(request.PPWebPath); err != nil {
		return finalizeCLIUpdateError(request.StatusPath, "rollback", now, err)
	}
	// Откат фронтенда
	if err := rollbackDirectory(request.FrontendDist); err != nil {
		return finalizeCLIUpdateError(request.StatusPath, "rollback", now, err)
	}

	if request.PPService != "" && serviceUnitExists(request.PPService) {
		_ = restartSystemService(request.PPService)
	}

	finishedAt := time.Now().UTC()
	_ = writeCLIUpdateStatus(request.StatusPath, updateRunStatus{
		State:      "success",
		Message:    "Откат успешно завершен. Система перезагружается.",
		StartedAt:  timePtr(now),
		FinishedAt: timePtr(finishedAt),
	})

	if request.WebService != "" && serviceUnitExists(request.WebService) {
		_ = restartSystemService(request.WebService)
	}

	return nil
}

func rollbackFile(path string) error {
	bak := path + ".bak"
	if _, err := os.Stat(bak); err != nil {
		return fmt.Errorf("резервная копия %s не найдена", bak)
	}
	// Мы не удаляем текущий файл, а меняем их местами, чтобы можно было "откатить откат" если что
	tmp := path + ".tmp-rollback"
	_ = os.Remove(tmp)
	if err := os.Rename(path, tmp); err != nil {
		return fmt.Errorf("не удалось переместить текущий файл %s: %w", path, err)
	}
	if err := os.Rename(bak, path); err != nil {
		_ = os.Rename(tmp, path)
		return fmt.Errorf("не удалось восстановить файл из %s: %w", bak, err)
	}
	_ = os.Rename(tmp, bak)
	return nil
}

func rollbackDirectory(path string) error {
	bak := path + ".bak"
	if _, err := os.Stat(bak); err != nil {
		return fmt.Errorf("резервная копия директории %s не найдена", bak)
	}
	tmp := path + ".tmp-rollback"
	_ = os.RemoveAll(tmp)
	if err := os.Rename(path, tmp); err != nil {
		return fmt.Errorf("не удалось переместить текущую директорию %s: %w", path, err)
	}
	if err := os.Rename(bak, path); err != nil {
		_ = os.Rename(tmp, path)
		return fmt.Errorf("не удалось восстановить директорию из %s: %w", bak, err)
	}
	_ = os.Rename(tmp, bak)
	return nil
}

func applyReleaseUpdate(request releaseApplyRequest) error {
	now := time.Now().UTC()
	if err := writeCLIUpdateStatus(request.StatusPath, updateRunStatus{
		State:         "running",
		Message:       fmt.Sprintf("Устанавливается релиз %s.", humanVersion(request.Tag)),
		TargetVersion: request.Tag,
		StartedAt:     timePtr(now),
	}); err != nil {
		return err
	}

	if request.Repo == "" {
		request.Repo = gitHubRepoSlug
	}
	if request.Tag == "" {
		request.Tag = "latest"
	}
	if request.PPPath == "" || request.PPWebPath == "" || request.FrontendDist == "" {
		return finalizeCLIUpdateError(request.StatusPath, request.Tag, now, fmt.Errorf("пути установки релиза заданы не полностью"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	release, err := fetchReleaseForInstall(ctx, request.Repo, request.Tag)
	if err != nil {
		return finalizeCLIUpdateError(request.StatusPath, request.Tag, now, err)
	}

	request.Tag = release.TagName
	if err := writeCLIUpdateStatus(request.StatusPath, updateRunStatus{
		State:         "running",
		Message:       fmt.Sprintf("Скачивается релиз %s.", humanVersion(request.Tag)),
		TargetVersion: request.Tag,
		StartedAt:     timePtr(now),
	}); err != nil {
		return err
	}

	ppAssetName := fmt.Sprintf("pp-core_linux_%s", runtime.GOARCH)
	ppWebAssetName := fmt.Sprintf("pp-web_linux_%s", runtime.GOARCH)
	frontendAssetName := "pp-web-frontend.tar.gz"

	ppAsset, err := findReleaseAsset(release, ppAssetName)
	if err != nil {
		return finalizeCLIUpdateError(request.StatusPath, request.Tag, now, err)
	}
	ppWebAsset, err := findReleaseAsset(release, ppWebAssetName)
	if err != nil {
		return finalizeCLIUpdateError(request.StatusPath, request.Tag, now, err)
	}
	frontendAsset, err := findReleaseAsset(release, frontendAssetName)
	if err != nil {
		return finalizeCLIUpdateError(request.StatusPath, request.Tag, now, err)
	}

	workDir, err := os.MkdirTemp("", "pp-web-update-*")
	if err != nil {
		return finalizeCLIUpdateError(request.StatusPath, request.Tag, now, err)
	}
	defer os.RemoveAll(workDir)

	ppDownload := filepath.Join(workDir, ppAssetName)
	ppWebDownload := filepath.Join(workDir, ppWebAssetName)
	frontendDownload := filepath.Join(workDir, frontendAssetName)

	if err := downloadReleaseAsset(ctx, ppAsset.BrowserDownloadURL, ppDownload, 0o755); err != nil {
		return finalizeCLIUpdateError(request.StatusPath, request.Tag, now, err)
	}
	if err := downloadReleaseAsset(ctx, ppWebAsset.BrowserDownloadURL, ppWebDownload, 0o755); err != nil {
		return finalizeCLIUpdateError(request.StatusPath, request.Tag, now, err)
	}
	if err := downloadReleaseAsset(ctx, frontendAsset.BrowserDownloadURL, frontendDownload, 0o644); err != nil {
		return finalizeCLIUpdateError(request.StatusPath, request.Tag, now, err)
	}

	if err := installReleaseBinary(ppDownload, request.PPPath); err != nil {
		return finalizeCLIUpdateError(request.StatusPath, request.Tag, now, err)
	}
	if err := installReleaseBinary(ppWebDownload, request.PPWebPath); err != nil {
		return finalizeCLIUpdateError(request.StatusPath, request.Tag, now, err)
	}
	if err := installFrontendBundle(frontendDownload, request.FrontendDist); err != nil {
		return finalizeCLIUpdateError(request.StatusPath, request.Tag, now, err)
	}

	finishedAt := time.Now().UTC()
	if err := writeCLIUpdateStatus(request.StatusPath, updateRunStatus{
		State:         "success",
		Message:       fmt.Sprintf("Релиз %s установлен.", humanVersion(request.Tag)),
		TargetVersion: request.Tag,
		StartedAt:     timePtr(now),
		FinishedAt:    timePtr(finishedAt),
	}); err != nil {
		return err
	}

	for _, svc := range []string{request.PPService, request.WebService} {
		if svc != "" && serviceUnitExists(svc) {
			_ = restartSystemService(svc)
		}
	}

	return nil
}

func finalizeCLIUpdateError(statusPath, targetVersion string, startedAt time.Time, cause error) error {
	finishedAt := time.Now().UTC()
	_ = writeCLIUpdateStatus(statusPath, updateRunStatus{
		State:         "error",
		Message:       cause.Error(),
		TargetVersion: targetVersion,
		StartedAt:     timePtr(startedAt),
		FinishedAt:    timePtr(finishedAt),
	})
	return cause
}

func writeCLIUpdateStatus(statusPath string, status updateRunStatus) error {
	if strings.TrimSpace(statusPath) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(statusPath), 0o755); err != nil {
		return err
	}
	return writeJSONFileAtomic(statusPath, status, 0o644)
}

func fetchReleaseForInstall(ctx context.Context, repo, tag string) (*gitHubRelease, error) {
	endpoint := gitHubLatestRelease
	if repo != gitHubRepoSlug {
		endpoint = fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	}
	if tag != "" && tag != "latest" {
		endpoint = gitHubReleaseByTagURL(repo, tag)
	}
	return fetchGitHubRelease(ctx, endpoint)
}

func findReleaseAsset(release *gitHubRelease, name string) (*gitHubReleaseAsset, error) {
	for i := range release.Assets {
		if release.Assets[i].Name == name {
			return &release.Assets[i], nil
		}
	}
	return nil, fmt.Errorf("в релизе %s не найден asset %s", release.TagName, name)
}

func downloadReleaseAsset(ctx context.Context, assetURL, destination string, mode os.FileMode) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, assetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "pp-web-updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("download failed: %s", strings.TrimSpace(string(body)))
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}

	tmpPath := destination + ".partial"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	if _, err := io.Copy(file, resp.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

func installReleaseBinary(sourcePath, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("скачанный бинарник %s пустой", sourcePath)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}

	// Создаем бэкап перед установкой
	if _, err := os.Stat(targetPath); err == nil {
		bakPath := targetPath + ".bak"
		_ = os.Remove(bakPath)
		if err := copyFile(targetPath, bakPath); err != nil {
			return fmt.Errorf("failed to create backup of %s: %w", targetPath, err)
		}
	}

	tempFile, err := os.CreateTemp(filepath.Dir(targetPath), filepath.Base(targetPath)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()

	cleanup := func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}

	if _, err := io.Copy(tempFile, source); err != nil {
		cleanup()
		return err
	}
	if err := tempFile.Chmod(0o755); err != nil {
		cleanup()
		return err
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func installFrontendBundle(archivePath, targetDir string) error {
	parentDir := filepath.Dir(targetDir)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return err
	}

	stagingDir, err := os.MkdirTemp(parentDir, ".pp-web-frontend-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stagingDir)

	extractedDir := filepath.Join(stagingDir, "dist")
	if err := os.MkdirAll(extractedDir, 0o755); err != nil {
		return err
	}
	if err := extractFrontendArchive(archivePath, extractedDir); err != nil {
		return err
	}

	backupDir := targetDir + ".bak"
	_ = os.RemoveAll(backupDir)
	if _, err := os.Stat(targetDir); err == nil {
		if err := os.Rename(targetDir, backupDir); err != nil {
			return err
		}
	}

	if err := os.Rename(extractedDir, targetDir); err != nil {
		if _, restoreErr := os.Stat(backupDir); restoreErr == nil {
			_ = os.Rename(backupDir, targetDir)
		}
		return err
	}

	return nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}

func extractFrontendArchive(archivePath, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		stripComponents := 0
		parts := strings.Split(strings.TrimPrefix(strings.ReplaceAll(header.Name, "\\", "/"), "./"), "/")
		if len(parts) > 1 && (parts[0] == "dist" || parts[0] == "frontend" || parts[0] == ".") {
			stripComponents = 1
		}
		relativePath := sanitizeArchivePath(header.Name, stripComponents)

		if relativePath == "" || relativePath == "." {
			continue
		}

		targetPath := filepath.Join(destination, relativePath)
		if !strings.HasPrefix(targetPath, destination+string(os.PathSeparator)) && targetPath != destination {
			return fmt.Errorf("archive path %s escapes destination", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			output, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(output, tarReader); err != nil {
				_ = output.Close()
				return err
			}
			if err := output.Close(); err != nil {
				return err
			}
		}
	}
}

func sanitizeArchivePath(name string, stripComponents int) string {
	name = strings.ReplaceAll(strings.TrimSpace(name), "\\", "/")
	name = strings.TrimPrefix(name, "./")
	if name == "" {
		return ""
	}

	parts := strings.Split(name, "/")
	if stripComponents > len(parts) {
		return ""
	}
	parts = parts[stripComponents:]

	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		clean = append(clean, part)
	}
	if len(clean) == 0 {
		return ""
	}
	return path.Join(clean...)
}

func restartSystemService(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "systemctl", "restart", "--no-block", name).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("не удалось перезапустить %s: %s", name, message)
	}

	return nil
}

func serviceUnitExists(name string) bool {
	unitPaths := []string{
		filepath.Join("/etc/systemd/system", name+".service"),
		filepath.Join("/lib/systemd/system", name+".service"),
		filepath.Join("/usr/lib/systemd/system", name+".service"),
	}
	for _, unitPath := range unitPaths {
		if _, err := os.Stat(unitPath); err == nil {
			return true
		}
	}
	return false
}
