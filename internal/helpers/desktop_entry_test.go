package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRewriteDesktopEntryFileRewritesAllowedSectionsAndPreservesTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	desktopPath := filepath.Join(dir, "my-app.desktop")

	initial := strings.Join([]string{
		"[Desktop Entry]",
		"Name=My App",
		"Exec=AppRun %U",
		"Icon=old-icon",
		"",
		"[Desktop Action NewWindow]",
		"Exec=\"/old/path\" --new-window",
		"Icon=action-icon",
		"",
		"[Not Desktop]",
		"Exec=should-not-change",
	}, "\n")

	if err := os.WriteFile(desktopPath, []byte(initial), 0o640); err != nil {
		t.Fatalf("failed to write test desktop file: %v", err)
	}

	execPath := filepath.Join(dir, "My App.AppImage")
	iconPath := filepath.Join(dir, "my-app.png")

	if err := RewriteDesktopEntryFile(desktopPath, execPath, iconPath); err != nil {
		t.Fatalf("RewriteDesktopEntryFile returned error: %v", err)
	}

	out, err := os.ReadFile(desktopPath)
	if err != nil {
		t.Fatalf("failed to read updated desktop file: %v", err)
	}
	content := string(out)

	if !strings.Contains(content, "Exec=\""+execPath+"\" %U") {
		t.Fatalf("expected desktop entry Exec to be rewritten with quoted path, got:\n%s", content)
	}
	if !strings.Contains(content, "Exec=\""+execPath+"\" --new-window") {
		t.Fatalf("expected desktop action Exec to keep arguments, got:\n%s", content)
	}
	if !strings.Contains(content, "Icon="+iconPath) {
		t.Fatalf("expected desktop entry Icon to be rewritten, got:\n%s", content)
	}
	if !strings.Contains(content, "[Desktop Action NewWindow]\nExec=\""+execPath+"\" --new-window\nIcon=action-icon") {
		t.Fatalf("expected action Icon to remain unchanged, got:\n%s", content)
	}
	if !strings.Contains(content, "[Not Desktop]\nExec=should-not-change") {
		t.Fatalf("expected Exec outside desktop sections to remain unchanged, got:\n%s", content)
	}
	if !strings.HasSuffix(content, "\n") {
		t.Fatal("expected rewritten desktop file to keep trailing newline")
	}

	info, err := os.Stat(desktopPath)
	if err != nil {
		t.Fatalf("failed to stat rewritten desktop file: %v", err)
	}
	if got := info.Mode().Perm() & 0o666; got != 0o640 {
		t.Fatalf("desktop file perms = %#o, want %#o", got, 0o640)
	}
}

func TestRewriteDesktopEntryFileSkipsIconRewriteWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	desktopPath := filepath.Join(dir, "my-app.desktop")

	if err := os.WriteFile(desktopPath, []byte("[Desktop Entry]\nExec=AppRun\nIcon=old-icon\n"), 0o644); err != nil {
		t.Fatalf("failed to write test desktop file: %v", err)
	}

	if err := RewriteDesktopEntryFile(desktopPath, filepath.Join(dir, "app.AppImage"), ""); err != nil {
		t.Fatalf("RewriteDesktopEntryFile returned error: %v", err)
	}

	out, err := os.ReadFile(desktopPath)
	if err != nil {
		t.Fatalf("failed to read updated desktop file: %v", err)
	}
	content := string(out)

	if !strings.Contains(content, "Exec="+filepath.Join(dir, "app.AppImage")) {
		t.Fatalf("expected exec rewrite, got:\n%s", content)
	}
	if !strings.Contains(content, "Icon=old-icon") {
		t.Fatalf("expected icon to remain unchanged, got:\n%s", content)
	}
}
