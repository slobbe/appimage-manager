package add

import (
	"path/filepath"
	"testing"
)

func TestNormalizeLocalAppImagePathReturnsAbsolutePath(t *testing.T) {
	got, err := normalizeLocalAppImagePath("Example.AppImage")
	if err != nil {
		t.Fatalf("normalizeLocalAppImagePath() error = %v", err)
	}

	want, err := filepath.Abs("Example.AppImage")
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	if got != want {
		t.Fatalf("normalizeLocalAppImagePath() = %q, want %q", got, want)
	}
}

func TestNormalizeLocalAppImagePathTrimsWhitespace(t *testing.T) {
	got, err := normalizeLocalAppImagePath("  ./Example.AppImage  ")
	if err != nil {
		t.Fatalf("normalizeLocalAppImagePath() error = %v", err)
	}

	want, err := filepath.Abs("./Example.AppImage")
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	if got != want {
		t.Fatalf("normalizeLocalAppImagePath() = %q, want %q", got, want)
	}
}

func TestNormalizeLocalAppImagePathExpandsHomeDirectory(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")

	got, err := normalizeLocalAppImagePath("~/Downloads/Example.AppImage")
	if err != nil {
		t.Fatalf("normalizeLocalAppImagePath() error = %v", err)
	}

	want := filepath.Join("/home/testuser", "Downloads", "Example.AppImage")
	if got != want {
		t.Fatalf("normalizeLocalAppImagePath() = %q, want %q", got, want)
	}
}

func TestNormalizeLocalAppImagePathRejectsEmptyPath(t *testing.T) {
	_, err := normalizeLocalAppImagePath("  ")
	if err == nil {
		t.Fatal("normalizeLocalAppImagePath() error = nil, want error")
	}
}
