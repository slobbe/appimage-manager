package appimage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallerInstallsAppImage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	source := filepath.Join(root, "source.AppImage")
	if err := os.WriteFile(source, []byte("appimage"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	destination, err := NewInstaller(filepath.Join(root, "library")).Install(context.Background(), source, "example")
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	if got, want := destination, filepath.Join(root, "library", "example.AppImage"); got != want {
		t.Fatalf("Install() = %q, want %q", got, want)
	}
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if got, want := string(content), "appimage"; got != want {
		t.Fatalf("destination content = %q, want %q", got, want)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source stat error = %v, want not exist", err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("stat destination: %v", err)
	}
	if info.Mode()&0o100 == 0 {
		t.Fatalf("destination mode = %v, want owner executable", info.Mode())
	}
}

func TestInstallerOverwritesExistingAppImage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	library := filepath.Join(root, "library")
	if err := os.MkdirAll(library, 0o755); err != nil {
		t.Fatalf("mkdir library: %v", err)
	}
	destination := filepath.Join(library, "example.AppImage")
	if err := os.WriteFile(destination, []byte("old"), 0o700); err != nil {
		t.Fatalf("write existing destination: %v", err)
	}
	source := filepath.Join(root, "source.AppImage")
	if err := os.WriteFile(source, []byte("new"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	installed, err := NewInstaller(library).Install(context.Background(), source, "example")
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
		name       string
		dir        string
		sourcePath string
		appID      string
	}{
		{name: "missing dir", dir: "", sourcePath: "source.AppImage", appID: "example"},
		{name: "missing source", dir: "library", sourcePath: "", appID: "example"},
		{name: "missing app id", dir: "library", sourcePath: "source.AppImage", appID: ""},
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

	_, err := NewInstaller("library").Install(ctx, "source.AppImage", "example")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Install() error = %v, want context.Canceled", err)
	}
}
