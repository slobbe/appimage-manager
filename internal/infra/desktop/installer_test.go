package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallerInstallsDesktopEntry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	content := []byte("[Desktop Entry]\nName=Example\n")

	destination, err := NewInstaller(filepath.Join(root, "applications")).Install(context.Background(), "example", content)
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if got, want := destination, filepath.Join(root, "applications", "example.desktop"); got != want {
		t.Fatalf("Install() = %q, want %q", got, want)
	}
	installed, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if got := string(installed); got != string(content) {
		t.Fatalf("destination content = %q, want %q", got, string(content))
	}
}

func TestInstallerOverwritesExistingDesktopEntry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "applications")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir applications: %v", err)
	}
	destination := filepath.Join(dir, "example.desktop")
	if err := os.WriteFile(destination, []byte("old"), 0o644); err != nil {
		t.Fatalf("write existing destination: %v", err)
	}

	installed, err := NewInstaller(dir).Install(context.Background(), "example", []byte("new"))
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if installed != destination {
		t.Fatalf("Install() = %q, want %q", installed, destination)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if got, want := string(content), "new"; got != want {
		t.Fatalf("destination content = %q, want %q", got, want)
	}
}

func TestInstallerValidatesInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dir     string
		appID   string
		content []byte
	}{
		{name: "missing dir", dir: "", appID: "example", content: []byte("content")},
		{name: "missing app id", dir: "applications", appID: "", content: []byte("content")},
		{name: "missing content", dir: "applications", appID: "example", content: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewInstaller(tt.dir).Install(context.Background(), tt.appID, tt.content)
			if err == nil {
				t.Fatal("Install() error = nil, want error")
			}
		})
	}
}

func TestInstallerRespectsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewInstaller("applications").Install(ctx, "example", []byte("content"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Install() error = %v, want context.Canceled", err)
	}
}
