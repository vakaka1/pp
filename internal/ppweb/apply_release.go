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

	if err := fs.Parse(args); err != nil {
		return err
	}

	return applyReleaseUpdate(request)
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

	ppAssetName := fmt.Sprintf("pp_linux_%s", runtime.GOARCH)
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

	if request.PPService != "" && serviceUnitExists(request.PPService) {
		if err := restartSystemService(request.PPService); err != nil {
			return finalizeCLIUpdateError(request.StatusPath, request.Tag, now, err)
		}
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

	if request.WebService != "" && serviceUnitExists(request.WebService) {
		if err := restartSystemService(request.WebService); err != nil {
			return finalizeCLIUpdateError(request.StatusPath, request.Tag, now, err)
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

	_ = os.RemoveAll(backupDir)
	return nil
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

		relativePath := sanitizeArchivePath(header.Name, 1)
		if relativePath == "" {
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

func sanitizeArchivePath(raw string, stripComponents int) string {
	raw = strings.ReplaceAll(raw, "\\", "/")
	parts := strings.Split(raw, "/")
	if len(parts) <= stripComponents {
		return ""
	}

	cleanParts := make([]string, 0, len(parts)-stripComponents)
	for _, part := range parts[stripComponents:] {
		part = strings.TrimSpace(part)
		switch part {
		case "", ".", "..":
			continue
		default:
			cleanParts = append(cleanParts, part)
		}
	}

	return filepath.Join(cleanParts...)
}

func restartSystemService(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "systemctl", "restart", name).CombinedOutput()
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
