package fileutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupManagerRestoresReplacedArtifact(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	backup, err := (BackupManager{}).Backup(context.Background(), []string{path})
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	defer func() {
		if err := backup.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	if err := os.WriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := backup.Restore(context.Background()); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "old" {
		t.Fatalf("restored content = %q, want old", content)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("restored mode = %o, want 755", info.Mode().Perm())
	}
}

func TestBackupManagerFindsExactDestinationThroughSymlinkedRoot(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	realRoot := filepath.Join(parent, "real")
	symlinkRoot := filepath.Join(parent, "linked")
	if err := os.Symlink(realRoot, symlinkRoot); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(realRoot, "hicolor", "256x256", "apps", "new-id.svg")
	if err := os.MkdirAll(filepath.Dir(existing), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(existing, []byte("existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(symlinkRoot, "hicolor", "256x256", "apps", "new-id.svg")
	paths, err := (BackupManager{}).Existing(context.Background(), []string{destination})
	if err != nil {
		t.Fatalf("Existing() error = %v", err)
	}
	if len(paths) != 1 || paths[0] != destination {
		t.Fatalf("Existing() = %#v, want %#v", paths, []string{destination})
	}
}

func TestBackupManagerExistingIncludesDanglingSymlink(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "dangling")
	if err := os.Symlink("missing-target", path); err != nil {
		t.Fatal(err)
	}
	paths, err := (BackupManager{}).Existing(context.Background(), []string{path})
	if err != nil {
		t.Fatalf("Existing() error = %v", err)
	}
	if len(paths) != 1 || paths[0] != path {
		t.Fatalf("Existing() = %#v, want dangling symlink", paths)
	}
}
