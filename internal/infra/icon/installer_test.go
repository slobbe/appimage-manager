package icon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallerInstallsIcon(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "example.SVG")
	if err := os.WriteFile(source, []byte("icon"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	destination, err := NewInstaller(filepath.Join(root, "icons")).Install(context.Background(), source, "example")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if got, want := destination, filepath.Join(root, "icons", "hicolor", "scalable", "apps", "example.svg"); got != want {
		t.Fatalf("Install() = %q, want %q", got, want)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if got, want := string(content), "icon"; got != want {
		t.Fatalf("destination content = %q, want %q", got, want)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source should remain after icon install: %v", err)
	}
}

func TestInstallerInstallsDirIconAsPng(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, ".DirIcon")
	if err := os.WriteFile(source, []byte("icon"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	destination, err := NewInstaller(filepath.Join(root, "icons")).Install(context.Background(), source, "example")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if got, want := destination, filepath.Join(root, "icons", "hicolor", "256x256", "apps", "example.png"); got != want {
		t.Fatalf("Install() = %q, want %q", got, want)
	}
}

func TestInstallerRejectsUnsupportedIcon(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "example.txt")
	if err := os.WriteFile(source, []byte("icon"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	_, err := NewInstaller(filepath.Join(root, "icons")).Install(context.Background(), source, "example")
	if err == nil {
		t.Fatal("Install() error = nil, want error")
	}
}

func TestInstallerValidatesInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		dir        string
		sourcePath string
		appID      string
	}{
		{name: "missing dir", dir: "", sourcePath: "icon.png", appID: "example"},
		{name: "missing source", dir: "icons", sourcePath: "", appID: "example"},
		{name: "missing app id", dir: "icons", sourcePath: "icon.png", appID: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewInstaller(tt.dir).Install(context.Background(), tt.sourcePath, tt.appID)
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

	_, err := NewInstaller("icons").Install(ctx, "icon.png", "example")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Install() error = %v, want context.Canceled", err)
	}
}
