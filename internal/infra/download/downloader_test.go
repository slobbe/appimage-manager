package download

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadWritesFileAndMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Last-Modified", "Wed, 13 May 2026 10:00:00 GMT")
		_, _ = w.Write([]byte("appimage"))
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "app.AppImage")
	var last Progress
	meta, err := (Downloader{Client: server.Client()}).Download(context.Background(), Request{
		URL:         server.URL,
		Destination: destination,
	}, func(progress Progress) {
		last = progress
	})
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}

	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(content) != "appimage" {
		t.Fatalf("downloaded content = %q, want %q", string(content), "appimage")
	}
	if meta.ETag != `"v1"` {
		t.Fatalf("metadata ETag = %q, want %q", meta.ETag, `"v1"`)
	}
	if last.Downloaded != int64(len("appimage")) {
		t.Fatalf("progress downloaded = %d, want %d", last.Downloaded, len("appimage"))
	}
}

func TestDownloadResumesWithRange(t *testing.T) {
	var sawRange string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRange = r.Header.Get("Range")
		if sawRange == "bytes=3-" {
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("def"))
			return
		}
		_, _ = w.Write([]byte("abcdef"))
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "app.AppImage")
	if err := os.WriteFile(destination, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (Downloader{Client: server.Client()}).Download(context.Background(), Request{
		URL:         server.URL,
		Destination: destination,
		Metadata:    &Metadata{URL: server.URL},
	}, nil)
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}

	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(content) != "abcdef" {
		t.Fatalf("downloaded content = %q, want %q", string(content), "abcdef")
	}
	if sawRange != "bytes=3-" {
		t.Fatalf("range header = %q, want %q", sawRange, "bytes=3-")
	}
}

func TestDownloadRestartsWhenServerRejectsRange(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 && r.Header.Get("Range") != "" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("abcdef"))
			return
		}
		_, _ = w.Write([]byte("abcdef"))
	}))
	defer server.Close()

	destination := filepath.Join(t.TempDir(), "app.AppImage")
	if err := os.WriteFile(destination, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (Downloader{Client: server.Client()}).Download(context.Background(), Request{
		URL:         server.URL,
		Destination: destination,
		Metadata:    &Metadata{URL: server.URL},
	}, nil)
	if err != nil {
		t.Fatalf("Download returned error: %v", err)
	}

	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}
	if string(content) != "abcdef" {
		t.Fatalf("downloaded content = %q, want %q", string(content), "abcdef")
	}
}

func TestDownloadReturnsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	_, err := (Downloader{Client: server.Client()}).Download(context.Background(), Request{
		URL:         server.URL,
		Destination: filepath.Join(t.TempDir(), "app.AppImage"),
	}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	statusErr, ok := err.(*StatusError)
	if !ok {
		t.Fatalf("error type = %T, want *StatusError", err)
	}
	if statusErr.Code != http.StatusBadGateway {
		t.Fatalf("status code = %d, want %d", statusErr.Code, http.StatusBadGateway)
	}
}
