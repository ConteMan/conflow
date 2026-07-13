package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunAlreadyLatest(t *testing.T) {
	server := newLocalHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/ConteMan/conflow/releases/latest" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"tag_name":"v1.2.3","assets":[]}`)
	}))

	var out bytes.Buffer
	err := Run(context.Background(), Options{
		CurrentVersion: "1.2.3",
		ExecutablePath: filepath.Join(t.TempDir(), "conflow"),
		APIBaseURL:     server.url,
		HTTPClient:     server.client,
		Out:            &out,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "already the latest") {
		t.Fatalf("output = %q, want already latest message", got)
	}
}

func TestRunCheckOnlyDoesNotDownload(t *testing.T) {
	server := newLocalHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/ConteMan/conflow/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v1.2.3","assets":[{"name":%q,"browser_download_url":"https://api.github.com/download/archive"}]}`,
				currentArchiveName("v1.2.3"))
		default:
			t.Fatalf("unexpected download during --check: %s", r.URL.Path)
		}
	}))

	var out bytes.Buffer
	err := Run(context.Background(), Options{
		CurrentVersion: "v1.0.0",
		ExecutablePath: filepath.Join(t.TempDir(), "conflow"),
		APIBaseURL:     server.url,
		HTTPClient:     server.client,
		CheckOnly:      true,
		Out:            &out,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Latest conflow version is v1.2.3") {
		t.Fatalf("output = %q, want latest version message", got)
	}
}

func TestRunDownloadAndReplace(t *testing.T) {
	binary := []byte("new conflow binary")
	archiveName := currentArchiveName("v1.2.3")
	archive := makeArchive(t, archiveName, binary)
	checksums := checksumLine(archiveName, archive)

	var out bytes.Buffer
	exePath := filepath.Join(t.TempDir(), exeName())
	if err := os.WriteFile(exePath, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("write old binary: %v", err)
	}

	server := newLocalHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/ConteMan/conflow/releases/latest":
			fmt.Fprintf(w, `{"tag_name":"v1.2.3","assets":[{"name":%q,"browser_download_url":"https://api.github.com/archive"},{"name":"checksums.txt","browser_download_url":"https://api.github.com/checksums"}]}`, archiveName)
		case "/archive":
			_, _ = w.Write(archive)
		case "/checksums":
			_, _ = w.Write([]byte(checksums))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))

	err := Run(context.Background(), Options{
		CurrentVersion: "v1.0.0",
		ExecutablePath: exePath,
		APIBaseURL:     server.url,
		HTTPClient:     server.client,
		Out:            &out,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "Updated conflow") {
		t.Fatalf("output = %q, want updated message", got)
	}
	got, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatalf("read replaced binary: %v", err)
	}
	if !bytes.Equal(got, binary) {
		t.Fatalf("replaced binary = %q, want %q", got, binary)
	}
}

func TestRunNetworkError(t *testing.T) {
	client := &http.Client{Transport: errorRoundTripper{}}
	err := Run(context.Background(), Options{
		CurrentVersion: "v1.0.0",
		ExecutablePath: filepath.Join(t.TempDir(), "conflow"),
		HTTPClient:     client,
		Out:            &bytes.Buffer{},
	})
	if err == nil {
		t.Fatal("expected error for network failure, got nil")
	}
}

func TestDetectInstallSource(t *testing.T) {
	cases := []struct {
		path string
		want InstallSource
	}{
		{"/usr/local/Cellar/conflow/1.0/bin/conflow", InstallSourceHomebrew},
		{"/opt/homebrew/bin/conflow", InstallSourceHomebrew},
		{`C:\Users\user\scoop\apps\conflow\current\conflow.exe`, InstallSourceScoop},
		{"/usr/local/bin/conflow", InstallSourceDirect},
	}
	for _, c := range cases {
		if got := DetectInstallSource(c.path); got != c.want {
			t.Errorf("DetectInstallSource(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func newLocalHTTPServer(handler http.Handler) localHTTPServer {
	return localHTTPServer{
		url: "https://api.github.com",
		client: &http.Client{
			Transport: localRoundTripper{handler: handler},
		},
	}
}

type localHTTPServer struct {
	url    string
	client *http.Client
}

type localRoundTripper struct {
	handler http.Handler
}

func (rt localRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	rt.handler.ServeHTTP(recorder, req)
	return recorder.Result(), nil
}

type errorRoundTripper struct{}

func (errorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network unavailable")
}

func currentArchiveName(version string) string {
	return fmt.Sprintf("conflow_%s_%s_%s%s", strings.TrimPrefix(version, "v"), runtime.GOOS, runtime.GOARCH, archiveSuffix(runtime.GOOS))
}

func makeArchive(t *testing.T, archiveName string, binary []byte) []byte {
	t.Helper()
	if strings.HasSuffix(archiveName, ".zip") {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, err := zw.Create(exeName())
		if err != nil {
			t.Fatalf("zip Create: %v", err)
		}
		if _, err := w.Write(binary); err != nil {
			t.Fatalf("zip Write: %v", err)
		}
		if err := zw.Close(); err != nil {
			t.Fatalf("zip Close: %v", err)
		}
		return buf.Bytes()
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: exeName(), Mode: 0o755, Size: int64(len(binary))}); err != nil {
		t.Fatalf("tar WriteHeader: %v", err)
	}
	if _, err := tw.Write(binary); err != nil {
		t.Fatalf("tar Write: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	return buf.Bytes()
}

func checksumLine(filename string, data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x  %s\n", sum[:], filename)
}

func exeName() string {
	if runtime.GOOS == "windows" {
		return "conflow.exe"
	}
	return "conflow"
}
