package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderCreatesWorkspace(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	workspace, err := NewProvider(baseDir).Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if !strings.HasPrefix(workspace.Path, baseDir+string(os.PathSeparator)) {
		t.Fatalf("Workspace.Path = %q, want under %q", workspace.Path, baseDir)
	}
	info, err := os.Stat(workspace.Path)
	if err != nil {
		t.Fatalf("stat workspace: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("workspace path is not a directory")
	}
}

func TestProviderCleanupRemovesWorkspace(t *testing.T) {
	t.Parallel()

	workspace, err := NewProvider(t.TempDir()).Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	file := filepath.Join(workspace.Path, "file.txt")
	if err := os.WriteFile(file, []byte("temporary"), 0o644); err != nil {
		t.Fatalf("write workspace file: %v", err)
	}

	if err := workspace.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, err := os.Stat(workspace.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat workspace after cleanup error = %v, want not exist", err)
	}
}

func TestProviderUsesDefaultTempDirWhenBaseDirEmpty(t *testing.T) {
	t.Parallel()

	workspace, err := NewProvider("").Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer workspace.Cleanup()

	if workspace.Path == "" {
		t.Fatal("Workspace.Path is empty")
	}
	if _, err := os.Stat(workspace.Path); err != nil {
		t.Fatalf("stat workspace: %v", err)
	}
}

func TestProviderUsesCustomPattern(t *testing.T) {
	t.Parallel()

	workspace, err := Provider{BaseDir: t.TempDir(), Pattern: "custom-*"}.Create(context.Background())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	defer workspace.Cleanup()

	if !strings.HasPrefix(filepath.Base(workspace.Path), "custom-") {
		t.Fatalf("Workspace.Path = %q, want custom pattern", workspace.Path)
	}
}

func TestProviderRespectsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewProvider(t.TempDir()).Create(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Create() error = %v, want context.Canceled", err)
	}
}

func TestProviderReturnsCreateError(t *testing.T) {
	t.Parallel()

	missingBaseDir := filepath.Join(t.TempDir(), "missing", "nested")
	_, err := NewProvider(missingBaseDir).Create(context.Background())
	if err == nil {
		t.Fatal("Create() error = nil, want error")
	}
}
