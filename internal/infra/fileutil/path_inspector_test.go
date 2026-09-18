package fileutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPathInspectorRecognizesSymlinkAliases(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realPath := filepath.Join(realDir, "example.AppImage")
	if err := os.WriteFile(realPath, []byte("app"), 0o755); err != nil {
		t.Fatal(err)
	}
	aliasDir := filepath.Join(root, "alias")
	if err := os.Symlink(realDir, aliasDir); err != nil {
		t.Fatal(err)
	}
	aliasPath := filepath.Join(aliasDir, "example.AppImage")

	same, err := (PathInspector{}).SameFile(context.Background(), realPath, aliasPath)
	if err != nil {
		t.Fatalf("SameFile() error = %v", err)
	}
	if !same {
		t.Fatalf("SameFile(%q, %q) = false, want true", realPath, aliasPath)
	}
}

func TestPathInspectorTreatsDanglingSymlinkAsExisting(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "dangling")
	if err := os.Symlink("missing-target", path); err != nil {
		t.Fatal(err)
	}
	exists, err := (PathInspector{}).Exists(context.Background(), path)
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Fatal("Exists() = false, want dangling symlink collision")
	}
}
