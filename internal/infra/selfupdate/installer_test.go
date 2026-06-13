package selfupdate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInstallerInstallRequiresVersionAndDoesNotDownloadScript(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		t.Fatalf("unexpected request for empty version")
	}))
	defer server.Close()

	installer := Installer{
		HTTPClient: server.Client(),
		ScriptURL:  server.URL,
		runCommand: func(context.Context, string, io.Reader, []string) ([]byte, error) {
			t.Fatalf("unexpected command execution for empty version")
			return nil, nil
		},
	}

	err := installer.Install(context.Background(), " \t\n")
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		t.Fatalf("unexpected request for canceled context")
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	installer := Installer{HTTPClient: server.Client(), ScriptURL: server.URL}
	err := installer.Install(ctx, "v0.18.0")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no script requests, got %d", requests)
	}
}

func TestInstallerInstallReturnsNon2xxDownloadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	defer server.Close()

	installer := Installer{
		HTTPClient: server.Client(),
		ScriptURL:  server.URL,
		runCommand: func(context.Context, string, io.Reader, []string) ([]byte, error) {
			t.Fatalf("unexpected command execution for failed download")
			return nil, nil
		},
	}

	err := installer.Install(context.Background(), "v0.18.0")
	if err == nil {
		t.Fatalf("expected non-2xx download error")
	}
	if !strings.Contains(err.Error(), "418") {
		t.Fatalf("expected status code in error, got %q", err.Error())
	}
}

func TestInstallerInstallRunsShellWithScriptAndTrimmedVersion(t *testing.T) {
	const scriptBody = "#!/bin/sh\necho installing\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, scriptBody)
	}))
	defer server.Close()

	commandCalled := false
	installer := Installer{
		HTTPClient: server.Client(),
		ScriptURL:  server.URL,
		runCommand: func(ctx context.Context, name string, stdin io.Reader, env []string) ([]byte, error) {
			commandCalled = true
			if err := ctx.Err(); err != nil {
				t.Fatalf("unexpected context error: %v", err)
			}
			if name != "sh" {
				t.Fatalf("expected command name sh, got %q", name)
			}
			body, err := io.ReadAll(stdin)
			if err != nil {
				t.Fatalf("read command stdin: %v", err)
			}
			if string(body) != scriptBody {
				t.Fatalf("expected stdin %q, got %q", scriptBody, string(body))
			}
			if !hasEnv(env, "AIM_VERSION=0.18.0") {
				t.Fatalf("expected AIM_VERSION=0.18.0 in env, got %#v", env)
			}
			return nil, nil
		},
	}

	if err := installer.Install(context.Background(), "v0.18.0"); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if !commandCalled {
		t.Fatalf("expected command runner to be called")
	}
}

func TestInstallerInstallIncludesCommandOutputOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "echo installing\n")
	}))
	defer server.Close()

	installer := Installer{
		HTTPClient: server.Client(),
		ScriptURL:  server.URL,
		runCommand: func(context.Context, string, io.Reader, []string) ([]byte, error) {
			return []byte("installer failed\n"), errors.New("exit status 1")
		},
	}

	err := installer.Install(context.Background(), "v0.18.0")
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "echo installing\n")
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	installer := Installer{
		HTTPClient: server.Client(),
		ScriptURL:  server.URL,
		runCommand: func(context.Context, string, io.Reader, []string) ([]byte, error) {
			cancel()
			return []byte("canceled\n"), errors.New("command canceled")
		},
	}

	err := installer.Install(ctx, "v0.18.0")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func hasEnv(env []string, want string) bool {
	for _, got := range env {
		if got == want {
			return true
		}
	}
	return false
}
