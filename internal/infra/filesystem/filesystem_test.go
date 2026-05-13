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

func TestLocateDesktopEntryFallsBackToRecursiveSearch(t *testing.T) {
	root := t.TempDir()

	preferred := filepath.Join(root, "usr", "share", "applications", "my-app.desktop")
	other := filepath.Join(root, "meta", "other.desktop")

	if err := os.MkdirAll(filepath.Dir(preferred), 0o755); err != nil {
		t.Fatalf("failed to create preferred desktop directory: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatalf("failed to create fallback desktop directory: %v", err)
	}
	if err := os.WriteFile(preferred, []byte("[Desktop Entry]\nName=Preferred\n"), 0o644); err != nil {
		t.Fatalf("failed to write preferred desktop file: %v", err)
	}
	if err := os.WriteFile(other, []byte("[Desktop Entry]\nName=Other\n"), 0o644); err != nil {
		t.Fatalf("failed to write fallback desktop file: %v", err)
	}

	got, err := LocateDesktopEntry(root)
	if err != nil {
		t.Fatalf("LocateDesktopEntry returned error: %v", err)
	}
	if got.Path != preferred {
		t.Fatalf("desktop path = %q, want %q", got.Path, preferred)
	}
	if got.Stem != "my-app" {
		t.Fatalf("desktop stem = %q, want %q", got.Stem, "my-app")
	}
}

func TestLocateIconResolvesSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.svg")
	linkPath := filepath.Join(root, "app.svg")

	if err := os.WriteFile(target, []byte("<svg/>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatal(err)
	}

	got, err := LocateIcon(root)
	if err != nil {
		t.Fatalf("LocateIcon returned error: %v", err)
	}
	if got != linkPath && got != target {
		t.Fatalf("icon path = %q, want %q or %q", got, linkPath, target)
	}
}

func TestWriteAtomicFileCreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "cache.json")

	if err := WriteAtomicFile(path, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatalf("WriteAtomicFile returned error: %v", err)
	}

	got, ok, err := ReadFileIfExists(path)
	if err != nil {
		t.Fatalf("ReadFileIfExists returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected written file to exist")
	}
	if string(got) != `{"ok":true}` {
		t.Fatalf("file content = %q, want %q", string(got), `{"ok":true}`)
	}
}
