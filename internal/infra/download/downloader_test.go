package download

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/app"
)

func TestDownloaderDownloadsFile(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("User-Agent"), "aim"; got != want {
			t.Fatalf("User-Agent = %q, want %q", got, want)
		}
		fmt.Fprint(w, "hello appimage")
	}))
	defer server.Close()

	root := t.TempDir()
	destination := filepath.Join(root, "nested", "Example.AppImage")
	progress := &fakeProgress{}
	downloader := NewDownloader()

	result, err := downloader.Download(context.Background(), app.DownloadSource{URL: server.URL}, destination, progress)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if got, want := result.Path, destination; got != want {
		t.Fatalf("DownloadedFile.Path = %q, want %q", got, want)
	}
	if got, want := result.SizeBytes, int64(len("hello appimage")); got != want {
		t.Fatalf("DownloadedFile.SizeBytes = %d, want %d", got, want)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if got, want := string(content), "hello appimage"; got != want {
		t.Fatalf("destination content = %q, want %q", got, want)
	}
	if got, want := progress.advanced, int64(len("hello appimage")); got != want {
		t.Fatalf("progress advanced = %d, want %d", got, want)
	}
	if _, err := os.Stat(destination + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file stat error = %v, want not exist", err)
	}
}

func TestDownloaderReturnsHTTPError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := (NewDownloader()).Download(context.Background(), app.DownloadSource{URL: server.URL}, filepath.Join(t.TempDir(), "file"), nil)
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("Download() error = %v, want 502 error", err)
	}
}

func TestDownloaderRejectsOversizedResponseBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "")
		fmt.Fprint(w, "too large")
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "Example.AppImage")
	_, err := (NewDownloader()).Download(context.Background(), app.DownloadSource{URL: server.URL, SizeBytes: 5}, destination, nil)
	if err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("Download() error = %v, want size error", err)
	}
	assertNoDownloadFiles(t, destination)
}

func TestDownloaderRejectsTruncatedResponseBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "short")
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "Example.AppImage")
	_, err := (NewDownloader()).Download(context.Background(), app.DownloadSource{URL: server.URL, SizeBytes: 20}, destination, nil)
	if err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("Download() error = %v, want size error", err)
	}
	assertNoDownloadFiles(t, destination)
}

func TestDownloaderRejectsMismatchedContentLength(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "10")
		fmt.Fprint(w, "1234567890")
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "Example.AppImage")
	_, err := (NewDownloader()).Download(context.Background(), app.DownloadSource{URL: server.URL, SizeBytes: 5}, destination, nil)
	if err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("Download() error = %v, want size error", err)
	}
	assertNoDownloadFiles(t, destination)
}

func TestDownloaderAllowsUnknownExpectedSize(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "unknown size")
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "Example.AppImage")
	result, err := (NewDownloader()).Download(context.Background(), app.DownloadSource{URL: server.URL, SizeBytes: 0}, destination, nil)
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if got, want := result.SizeBytes, int64(len("unknown size")); got != want {
		t.Fatalf("DownloadedFile.SizeBytes = %d, want %d", got, want)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if got, want := string(content), "unknown size"; got != want {
		t.Fatalf("destination content = %q, want %q", got, want)
	}
}

func TestDownloaderValidatesInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		source      app.DownloadSource
		destination string
	}{
		{name: "missing url", source: app.DownloadSource{}, destination: "file"},
		{name: "missing destination", source: app.DownloadSource{URL: "https://example.test/file"}, destination: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewDownloader().Download(context.Background(), tt.source, tt.destination, nil)
			if err == nil {
				t.Fatal("Download() error = nil, want error")
			}
		})
	}
}

func TestDownloaderRemovesTemporaryFileOnCanceledContext(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "partial")
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	progress := cancelingProgress{cancel: cancel}
	destination := filepath.Join(t.TempDir(), "Example.AppImage")

	_, err := (NewDownloader()).Download(ctx, app.DownloadSource{URL: server.URL}, destination, progress)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Download() error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(destination + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination stat error = %v, want not exist", err)
	}
}

func assertNoDownloadFiles(t *testing.T, destination string) {
	t.Helper()

	if _, err := os.Stat(destination + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary file stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination stat error = %v, want not exist", err)
	}
}

type fakeProgress struct {
	advanced int64
	current  int64
}

func (p *fakeProgress) Advance(delta int64) {
	p.advanced += delta
}

func (p *fakeProgress) Set(current int64) {
	p.current = current
}

type cancelingProgress struct {
	cancel context.CancelFunc
}

func (p cancelingProgress) Advance(delta int64) {
	p.cancel()
}

func (p cancelingProgress) Set(current int64) {}
