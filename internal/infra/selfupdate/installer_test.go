package selfupdate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestInstallerInstallRequiresVersionAndDoesNotDownloadScript(t *testing.T) {
	requests := 0
	withDefaultTransport(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		t.Fatalf("unexpected request for empty version")
		return nil, nil
	}))

	err := (Installer{}).Install(context.Background(), " \t\n")
	if err == nil {
		t.Fatalf("expected error for empty version")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Fatalf("expected version error, got %q", err.Error())
	}
	if requests != 0 {
		t.Fatalf("expected no script requests, got %d", requests)
	}
}

func TestInstallerInstallReturnsCanceledContextBeforeWork(t *testing.T) {
	requests := 0
	withDefaultTransport(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		t.Fatalf("unexpected request for canceled context")
		return nil, nil
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := (Installer{}).Install(ctx, "v0.18.0")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no script requests, got %d", requests)
	}
}

func TestInstallScriptURLForVersionUsesTaggedScript(t *testing.T) {
	got := installScriptURLForVersion("v0.18.0")
	want := "https://raw.githubusercontent.com/slobbe/appimage-manager/v0.18.0/scripts/install.sh"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestInstallScriptURLForVersionNormalizesVersionWithoutV(t *testing.T) {
	got := installScriptURLForVersion("0.18.0")
	want := "https://raw.githubusercontent.com/slobbe/appimage-manager/v0.18.0/scripts/install.sh"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestInstallerInstallDownloadsTaggedScript(t *testing.T) {
	requestedURL := ""
	withInstallScript(t, "echo installing\n", http.StatusOK, &requestedURL)

	if err := (Installer{}).Install(context.Background(), "v0.18.0"); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	want := "https://raw.githubusercontent.com/slobbe/appimage-manager/v0.18.0/scripts/install.sh"
	if requestedURL != want {
		t.Fatalf("request URL = %q, want %q", requestedURL, want)
	}
}

func TestInstallerInstallReturnsNon2xxDownloadError(t *testing.T) {
	withInstallScript(t, "nope", http.StatusTeapot, nil)

	err := (Installer{}).Install(context.Background(), "v0.18.0")
	if err == nil {
		t.Fatalf("expected non-2xx download error")
	}
	if !strings.Contains(err.Error(), "teapot") {
		t.Fatalf("expected status in error, got %q", err.Error())
	}
}

func TestInstallerInstallRunsShellWithScriptAndTrimmedVersion(t *testing.T) {
	logPath := t.TempDir() + "/installer.log"
	script := "printf 'version=%s\\n' \"$AIM_VERSION\" > " + shellQuote(logPath) + "\n"
	withInstallScript(t, script, http.StatusOK, nil)

	if err := (Installer{}).Install(context.Background(), "v0.18.0"); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	content, err := io.ReadAll(mustOpen(t, logPath))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if got, want := string(content), "version=0.18.0\n"; got != want {
		t.Fatalf("installer log = %q, want %q", got, want)
	}
}

func TestInstallerInstallIncludesCommandOutputOnFailure(t *testing.T) {
	withInstallScript(t, "echo installer failed\nexit 7\n", http.StatusOK, nil)

	err := (Installer{}).Install(context.Background(), "v0.18.0")
	if err == nil {
		t.Fatalf("expected command failure")
	}
	if !strings.Contains(err.Error(), "run self-update installer") {
		t.Fatalf("expected command failure context, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "installer failed") {
		t.Fatalf("expected command output in error, got %q", err.Error())
	}
}

func TestInstallerInstallReturnsCanceledContextDuringCommand(t *testing.T) {
	withInstallScript(t, "sleep 1\n", http.StatusOK, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := (Installer{}).Install(ctx, "v0.18.0")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func withInstallScript(t *testing.T, body string, status int, requestedURL *string) {
	t.Helper()
	withDefaultTransport(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if requestedURL != nil {
			*requestedURL = r.URL.String()
		}
		if got, want := r.Header.Get("User-Agent"), "aim"; got != want {
			t.Fatalf("User-Agent = %q, want %q", got, want)
		}
		return &http.Response{
			StatusCode: status,
			Status:     http.StatusText(status),
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
			Request:    r,
		}, nil
	}))
}

func withDefaultTransport(t *testing.T, transport http.RoundTripper) {
	t.Helper()
	previous := http.DefaultTransport
	http.DefaultTransport = transport
	t.Cleanup(func() { http.DefaultTransport = previous })
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func mustOpen(t *testing.T, path string) io.Reader {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { file.Close() })
	return file
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
