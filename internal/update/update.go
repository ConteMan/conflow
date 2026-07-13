// Package update implements explicit user-triggered conflow upgrades.
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	defaultAPIBaseURL = "https://api.github.com"
	repoPath          = "repos/ConteMan/conflow"
	userAgent         = "conflow-updater"
)

type InstallSource string

const (
	InstallSourceDirect   InstallSource = "direct"
	InstallSourceHomebrew InstallSource = "homebrew"
	InstallSourceScoop    InstallSource = "scoop"
)

type Options struct {
	CurrentVersion string
	ExecutablePath string
	APIBaseURL     string
	HTTPClient     *http.Client
	CheckOnly      bool
	Out            io.Writer
}

type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func Run(ctx context.Context, opts Options) error {
	exePath := opts.ExecutablePath
	if exePath == "" {
		var err error
		exePath, err = os.Executable()
		if err != nil {
			return fmt.Errorf("could not locate current executable: %w", err)
		}
		if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
			exePath = resolved
		}
	}

	out := opts.Out
	if out == nil {
		out = io.Discard
	}

	switch DetectInstallSource(exePath) {
	case InstallSourceHomebrew:
		fmt.Fprintln(out, "This conflow binary appears to be managed by Homebrew. Run: brew upgrade conflow")
		return nil
	case InstallSourceScoop:
		fmt.Fprintln(out, "This conflow binary appears to be managed by Scoop. Run: scoop update conflow")
		return nil
	}

	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	current := strings.TrimSpace(opts.CurrentVersion)
	if current == "" {
		current = "dev"
	}

	rel, err := fetchLatestRelease(ctx, client, opts.apiBaseURL())
	if err != nil {
		return err
	}
	latest := strings.TrimSpace(rel.TagName)
	if latest == "" {
		return errors.New("could not check latest release: GitHub returned a release without tag_name")
	}
	if sameVersion(current, latest) {
		fmt.Fprintf(out, "conflow %s is already the latest version.\n", latest)
		return nil
	}
	if opts.CheckOnly {
		fmt.Fprintf(out, "Latest conflow version is %s (current %s).\n", latest, current)
		return nil
	}

	archiveAsset, checksumAsset, err := selectAssets(rel)
	if err != nil {
		return err
	}
	archiveData, err := download(ctx, client, archiveAsset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("could not download %s: %w", archiveAsset.Name, err)
	}
	checksumData, err := download(ctx, client, checksumAsset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("could not download %s: %w", checksumAsset.Name, err)
	}
	if err := verifyChecksum(archiveAsset.Name, archiveData, checksumData); err != nil {
		return err
	}
	binaryData, err := extractBinary(archiveAsset.Name, archiveData)
	if err != nil {
		return err
	}
	if err := replaceExecutable(exePath, binaryData); err != nil {
		return fmt.Errorf("could not replace executable: %w", err)
	}
	fmt.Fprintf(out, "Updated conflow from %s to %s.\n", current, latest)
	return nil
}

func (o Options) apiBaseURL() string {
	if strings.TrimSpace(o.APIBaseURL) == "" {
		return defaultAPIBaseURL
	}
	return strings.TrimRight(o.APIBaseURL, "/")
}

func DetectInstallSource(exePath string) InstallSource {
	normalized := strings.ToLower(strings.ReplaceAll(exePath, "\\", "/"))
	switch {
	case strings.Contains(normalized, "/cellar/") || strings.Contains(normalized, "/homebrew/"):
		return InstallSourceHomebrew
	case strings.Contains(normalized, "/scoop/apps/") || strings.Contains(normalized, "/scoop/shims/"):
		return InstallSourceScoop
	default:
		return InstallSourceDirect
	}
}

func fetchLatestRelease(ctx context.Context, client *http.Client, apiBaseURL string) (releaseInfo, error) {
	url := apiBaseURL + "/" + repoPath + "/releases/latest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return releaseInfo{}, fmt.Errorf("could not check latest release: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)
	data, err := doBytes(client, req)
	if err != nil {
		return releaseInfo{}, fmt.Errorf("could not check latest release: %w", err)
	}
	var rel releaseInfo
	if err := json.Unmarshal(data, &rel); err != nil {
		return releaseInfo{}, fmt.Errorf("could not parse latest release response: %w", err)
	}
	return rel, nil
}

func download(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	if err := validateDownloadURL(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	return doBytes(client, req)
}

// validateDownloadURL restricts asset downloads to GitHub domains only.
func validateDownloadURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid download URL %q: %w", rawURL, err)
	}
	if u.Scheme != "https" && !isLoopbackHost(u.Hostname()) {
		return fmt.Errorf("refusing non-HTTPS download URL %q", rawURL)
	}
	host := u.Hostname()
	if host == "github.com" || strings.HasSuffix(host, ".github.com") ||
		strings.HasSuffix(host, ".githubusercontent.com") || isLoopbackHost(host) {
		return nil
	}
	return fmt.Errorf("refusing download from unexpected host %q", host)
}

func isLoopbackHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func doBytes(client *http.Client, req *http.Request) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s failed: %w", req.URL.String(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("GET %s returned %s", req.URL.String(), resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	return data, nil
}

func sameVersion(a, b string) bool {
	return strings.TrimPrefix(strings.TrimSpace(a), "v") == strings.TrimPrefix(strings.TrimSpace(b), "v")
}

func selectAssets(rel releaseInfo) (releaseAsset, releaseAsset, error) {
	var archiveAsset releaseAsset
	var checksumAsset releaseAsset
	suffix := archiveSuffix(runtime.GOOS)
	for _, asset := range rel.Assets {
		name := strings.ToLower(asset.Name)
		switch {
		case asset.Name == "checksums.txt" || strings.HasSuffix(name, "checksums.txt"):
			checksumAsset = asset
		case strings.HasSuffix(name, suffix) &&
			strings.Contains(name, runtime.GOOS) &&
			strings.Contains(name, runtime.GOARCH):
			archiveAsset = asset
		}
	}
	if archiveAsset.Name == "" {
		return releaseAsset{}, releaseAsset{}, fmt.Errorf("release %s has no archive for %s/%s", rel.TagName, runtime.GOOS, runtime.GOARCH)
	}
	if checksumAsset.Name == "" {
		return releaseAsset{}, releaseAsset{}, fmt.Errorf("release %s has no checksums.txt asset", rel.TagName)
	}
	return archiveAsset, checksumAsset, nil
}

func archiveSuffix(goos string) string {
	if goos == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

func verifyChecksum(filename string, data, checksumData []byte) error {
	want, ok := checksumFor(filename, checksumData)
	if !ok {
		return fmt.Errorf("checksums.txt does not contain %s", filename)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", filename, got, want)
	}
	return nil
}

func checksumFor(filename string, checksumData []byte) (string, bool) {
	for _, line := range strings.Split(string(checksumData), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if name == filename {
			return fields[0], true
		}
	}
	return "", false
}

func extractBinary(filename string, data []byte) ([]byte, error) {
	if strings.HasSuffix(strings.ToLower(filename), ".zip") {
		return extractZipBinary(data)
	}
	return extractTarGzBinary(data)
}

func extractTarGzBinary(data []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open tar.gz archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar.gz archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg || !isConflowBinaryName(header.Name) {
			continue
		}
		return io.ReadAll(tr)
	}
	return nil, errors.New("archive does not contain conflow binary")
}

func extractZipBinary(data []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open zip archive: %w", err)
	}
	for _, file := range zr.File {
		if file.FileInfo().IsDir() || !isConflowBinaryName(file.Name) {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			return nil, fmt.Errorf("read %s from zip archive: %w", file.Name, err)
		}
		data, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		return data, nil
	}
	return nil, errors.New("archive does not contain conflow binary")
}

func isConflowBinaryName(name string) bool {
	base := strings.ToLower(filepath.Base(name))
	return base == "conflow" || base == "conflow.exe"
}

func replaceExecutable(exePath string, binaryData []byte) error {
	dir := filepath.Dir(exePath)
	tmp, err := os.CreateTemp(dir, ".conflow-upgrade-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(binaryData); err != nil {
		_ = tmp.Close()
		return err
	}
	mode := os.FileMode(0o755)
	if info, err := os.Stat(exePath); err == nil {
		mode = info.Mode().Perm()
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		oldPath := exePath + ".old"
		_ = os.Remove(oldPath)
		if err := os.Rename(exePath, oldPath); err != nil {
			return err
		}
		if err := os.Rename(tmpPath, exePath); err != nil {
			if rollbackErr := os.Rename(oldPath, exePath); rollbackErr != nil {
				return fmt.Errorf("%w; rollback also failed (%v): previous binary left at %s", err, rollbackErr, oldPath)
			}
			return err
		}
		cleanup = false
		return nil
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		return err
	}
	cleanup = false
	return nil
}
