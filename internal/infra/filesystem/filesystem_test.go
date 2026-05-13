package filesystem

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyCreatesParentDirectory(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "source.AppImage")
	dst := filepath.Join(tmp, "nested", "app.AppImage")

	if err := os.WriteFile(src, []byte("appimage"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Copy(src, dst)
	if err != nil {
		t.Fatalf("Copy returned error: %v", err)
	}
	if got != dst {
		t.Fatalf("Copy returned %q, want %q", got, dst)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("failed to read copied file: %v", err)
	}
	if string(content) != "appimage" {
		t.Fatalf("copied content = %q, want %q", string(content), "appimage")
	}
}

func TestReplaceSymlinkReplacesExistingLink(t *testing.T) {
	tmp := t.TempDir()
	first := filepath.Join(tmp, "first.desktop")
	second := filepath.Join(tmp, "second.desktop")
	linkPath := filepath.Join(tmp, "links", "app.desktop")

	if err := os.WriteFile(first, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ReplaceSymlink(first, linkPath); err != nil {
		t.Fatalf("ReplaceSymlink returned error: %v", err)
	}
	if err := ReplaceSymlink(second, linkPath); err != nil {
		t.Fatalf("ReplaceSymlink returned error: %v", err)
	}

	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("failed to read symlink: %v", err)
	}
	if target != second {
		t.Fatalf("symlink target = %q, want %q", target, second)
	}
}

func TestReadTextFileRejectsDirectory(t *testing.T) {
	_, err := ReadTextFile(t.TempDir())
	if err == nil {
		t.Fatal("expected error for directory path")
	}
}

func TestResolveRegularFileResolvesSymlink(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "target.desktop")
	linkPath := filepath.Join(tmp, "link.desktop")

	if err := os.WriteFile(target, []byte("[Desktop Entry]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveRegularFile(linkPath, "desktop file")
	if err != nil {
		t.Fatalf("ResolveRegularFile returned error: %v", err)
	}
	if got != target {
		t.Fatalf("resolved path = %q, want %q", got, target)
	}
}
