package zsync

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchMetadataReturnsBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Filename: app.AppImage\n"))
	}))
	defer server.Close()

	metadata, err := (Client{HTTPClient: server.Client()}).FetchMetadata(server.URL + "/app.AppImage.zsync")
	if err != nil {
		t.Fatalf("FetchMetadata returned error: %v", err)
	}
	if string(metadata) != "Filename: app.AppImage\n" {
		t.Fatalf("metadata = %q", string(metadata))
	}
}

func TestFetchMetadataRejectsFailedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server failed", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := (Client{HTTPClient: server.Client()}).FetchMetadata(server.URL + "/app.AppImage.zsync")
	if err == nil {
		t.Fatal("expected status error")
	}
	if !strings.Contains(err.Error(), "status 500 Internal Server Error") {
		t.Fatalf("error = %q, want status substring", err.Error())
	}
}

func TestFetchMetadataRejectsOversizedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("a", MetadataMaxBytes+1)))
	}))
	defer server.Close()

	_, err := (Client{HTTPClient: server.Client()}).FetchMetadata(server.URL + "/app.AppImage.zsync")
	if err == nil {
		t.Fatal("expected oversized metadata error")
	}
	if !strings.Contains(err.Error(), "zsync metadata exceeds") {
		t.Fatalf("error = %q, want oversized substring", err.Error())
	}
}

func TestFetchMetadataUsesConfiguredHTTPClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte("Filename: app.AppImage\n"))
	}))
	defer server.Close()

	client := server.Client()
	client.Timeout = 20 * time.Millisecond

	start := time.Now()
	_, err := (Client{HTTPClient: client}).FetchMetadata(server.URL + "/app.AppImage.zsync")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed >= time.Second {
		t.Fatalf("elapsed = %s, want less than 1s", elapsed)
	}
	if !strings.Contains(err.Error(), "Client.Timeout") && !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("error = %q, want timeout substring", err.Error())
	}
}
