package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/slobbe/appimage-manager/internal/config"
)

func TestUpdateDesktopEntryRewritesExecAndIconInAllowedSections(t *testing.T) {
	dir := t.TempDir()
	desktopPath := filepath.Join(dir, "my-app.desktop")
	existingTryExec := filepath.Join(dir, "existing-binary")

	initial := strings.Join([]string{
		"[Desktop Entry]",
		"Name=My App",
		"TryExec=/missing/binary",
		"TryExec=ghostty",
		"TryExec=" + existingTryExec,
		"DBusActivatable=true",
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

	if err := os.WriteFile(existingTryExec, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to write existing TryExec target: %v", err)
	}

	if err := os.WriteFile(desktopPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("failed to write test desktop file: %v", err)
	}

	execPath := filepath.Join(dir, "My App.AppImage")
	iconPath := filepath.Join(dir, "my-app.png")

	if err := UpdateDesktopEntry(context.Background(), desktopPath, execPath, iconPath); err != nil {
		t.Fatalf("UpdateDesktopEntry returned error: %v", err)
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
	if strings.Contains(content, "TryExec=/missing/binary") {
		t.Fatalf("expected invalid absolute TryExec to be removed, got:\n%s", content)
	}
	if !strings.Contains(content, "TryExec=ghostty") {
		t.Fatalf("expected bare-command TryExec to remain unchanged, got:\n%s", content)
	}
	if !strings.Contains(content, "TryExec="+existingTryExec) {
		t.Fatalf("expected existing absolute TryExec to remain unchanged, got:\n%s", content)
	}
	if strings.Contains(content, "DBusActivatable=true") {
		t.Fatalf("expected DBusActivatable to be removed, got:\n%s", content)
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
}

func TestUpdateDesktopEntryRejectsNonAppImageExec(t *testing.T) {
	dir := t.TempDir()
	desktopPath := filepath.Join(dir, "my-app.desktop")

	if err := os.WriteFile(desktopPath, []byte("[Desktop Entry]\nExec=AppRun\n"), 0o644); err != nil {
		t.Fatalf("failed to write test desktop file: %v", err)
	}

	err := UpdateDesktopEntry(context.Background(), desktopPath, filepath.Join(dir, "binary"), filepath.Join(dir, "icon.png"))
	if err == nil {
		t.Fatal("expected error for non-AppImage exec path")
	}
	if !strings.Contains(err.Error(), ".AppImage") {
		t.Fatalf("error = %q, want AppImage hint", err.Error())
	}
}

func TestGetAppInfoFallsBackToVersionFromFilename(t *testing.T) {
	dir := t.TempDir()
	desktopPath := filepath.Join(dir, "0ad-0.28.0-x86_64.desktop")

	content := strings.Join([]string{
		"[Desktop Entry]",
		"Name=0 A.D.",
		"X-AppImage-Version=n/a",
	}, "\n") + "\n"

	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write desktop file: %v", err)
	}

	appInfo, err := GetAppInfo(context.Background(), desktopPath)
	if err != nil {
		t.Fatalf("GetAppInfo returned error: %v", err)
	}
	if appInfo.Version != "0.28.0" {
		t.Fatalf("appInfo.Version = %q, want %q", appInfo.Version, "0.28.0")
	}
}

func TestGetAppInfoFallsBackToNormalizedPlatformSuffixedFilename(t *testing.T) {
	dir := t.TempDir()
	desktopPath := filepath.Join(dir, "LocalSend-1.17.0-linux-x86-64.desktop")

	content := strings.Join([]string{
		"[Desktop Entry]",
		"Name=LocalSend",
		"X-AppImage-Version=n/a",
	}, "\n") + "\n"

	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write desktop file: %v", err)
	}

	appInfo, err := GetAppInfo(context.Background(), desktopPath)
	if err != nil {
		t.Fatalf("GetAppInfo returned error: %v", err)
	}
	if appInfo.Version != "1.17.0" {
		t.Fatalf("appInfo.Version = %q, want %q", appInfo.Version, "1.17.0")
	}
}

func TestGetAppInfoUsesUnknownWhenVersionUnavailable(t *testing.T) {
	dir := t.TempDir()
	desktopPath := filepath.Join(dir, "my-app.desktop")

	content := strings.Join([]string{
		"[Desktop Entry]",
		"Name=My App",
	}, "\n") + "\n"

	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write desktop file: %v", err)
	}

	appInfo, err := GetAppInfo(context.Background(), desktopPath)
	if err != nil {
		t.Fatalf("GetAppInfo returned error: %v", err)
	}
	if appInfo.Version != "unknown" {
		t.Fatalf("appInfo.Version = %q, want %q", appInfo.Version, "unknown")
	}
}

func TestGetAppInfoNormalizesDecoratedVersion(t *testing.T) {
	dir := t.TempDir()
	desktopPath := filepath.Join(dir, "standard-notes.desktop")

	content := strings.Join([]string{
		"[Desktop Entry]",
		"Name=Standard Notes",
		"X-AppImage-Version=@standardnotes/desktop@3.201.19",
	}, "\n") + "\n"

	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write desktop file: %v", err)
	}

	appInfo, err := GetAppInfo(context.Background(), desktopPath)
	if err != nil {
		t.Fatalf("GetAppInfo returned error: %v", err)
	}
	if appInfo.Version != "3.201.19" {
		t.Fatalf("appInfo.Version = %q, want %q", appInfo.Version, "3.201.19")
	}
}

func TestGetAppInfoNormalizesPlatformSuffixedMetadataVersion(t *testing.T) {
	dir := t.TempDir()
	desktopPath := filepath.Join(dir, "localsend.desktop")

	content := strings.Join([]string{
		"[Desktop Entry]",
		"Name=LocalSend",
		"X-AppImage-Version=1.17.0-linux-x86-64",
	}, "\n") + "\n"

	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write desktop file: %v", err)
	}

	appInfo, err := GetAppInfo(context.Background(), desktopPath)
	if err != nil {
		t.Fatalf("GetAppInfo returned error: %v", err)
	}
	if appInfo.Version != "1.17.0" {
		t.Fatalf("appInfo.Version = %q, want %q", appInfo.Version, "1.17.0")
	}
}

func TestGetAppInfoPrefersDesktopStemForID(t *testing.T) {
	dir := t.TempDir()
	desktopPath := filepath.Join(dir, "t3-code-desktop.desktop")

	content := strings.Join([]string{
		"[Desktop Entry]",
		"Name=T3 Code (Alpha)",
		"X-AppImage-Version=0.0.14",
	}, "\n") + "\n"

	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write desktop file: %v", err)
	}

	appInfo, err := GetAppInfo(context.Background(), desktopPath)
	if err != nil {
		t.Fatalf("GetAppInfo returned error: %v", err)
	}
	if appInfo.DesktopStem != "t3-code-desktop" {
		t.Fatalf("appInfo.DesktopStem = %q, want %q", appInfo.DesktopStem, "t3-code-desktop")
	}
	if appInfo.ID != "t3-code-desktop" {
		t.Fatalf("appInfo.ID = %q, want %q", appInfo.ID, "t3-code-desktop")
	}
}

func TestGetAppInfoFallsBackToSlugifiedNameWhenDesktopStemInvalid(t *testing.T) {
	dir := t.TempDir()
	desktopPath := filepath.Join(dir, "---.desktop")

	content := strings.Join([]string{
		"[Desktop Entry]",
		"Name=My App",
		"X-AppImage-Version=1.0.0",
	}, "\n") + "\n"

	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write desktop file: %v", err)
	}

	appInfo, err := GetAppInfo(context.Background(), desktopPath)
	if err != nil {
		t.Fatalf("GetAppInfo returned error: %v", err)
	}
	if appInfo.DesktopStem != "" {
		t.Fatalf("appInfo.DesktopStem = %q, want empty", appInfo.DesktopStem)
	}
	if appInfo.ID != "my-app" {
		t.Fatalf("appInfo.ID = %q, want %q", appInfo.ID, "my-app")
	}
}

func TestExtractAppImageResolvesDesktopSymlinkSource(t *testing.T) {
	tmp := t.TempDir()
	setupExtractionConfigForTest(t, tmp)

	appImagePath := filepath.Join(tmp, "0ad-0.28.0-x86_64.AppImage")
	writeFakeAppImageExtractor(t, appImagePath)

	extractionData, err := ExtractAppImage(context.Background(), appImagePath)
	if err != nil {
		t.Fatalf("ExtractAppImage returned error: %v", err)
	}

	info, err := os.Lstat(extractionData.DesktopEntryPath)
	if err != nil {
		t.Fatalf("expected extracted desktop file to exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("expected extracted desktop file to be materialized, got symlink: %s", extractionData.DesktopEntryPath)
	}

	appInfo, err := GetAppInfo(context.Background(), extractionData.DesktopEntryPath)
	if err != nil {
		t.Fatalf("GetAppInfo returned error for extracted desktop file: %v", err)
	}
	if appInfo.Name != "0 A.D." {
		t.Fatalf("appInfo.Name = %q, want %q", appInfo.Name, "0 A.D.")
	}
}

func TestExtractAppImageReportsDesktopLookupFailure(t *testing.T) {
	tmp := t.TempDir()
	setupExtractionConfigForTest(t, tmp)

	appImagePath := filepath.Join(tmp, "missing-desktop.AppImage")
	writeFakeAppImageExtractorWithoutDesktop(t, appImagePath)

	_, err := ExtractAppImage(context.Background(), appImagePath)
	if err == nil {
		t.Fatal("expected extraction error when no desktop file is present")
	}
	if !strings.Contains(err.Error(), "failed to locate desktop file") {
		t.Fatalf("error = %q, want desktop lookup error", err.Error())
	}
}

func TestExtractAppImageReturnsPromptlyWhenContextCanceled(t *testing.T) {
	tmp := t.TempDir()
	setupExtractionConfigForTest(t, tmp)

	appImagePath := filepath.Join(tmp, "slow.AppImage")
	writeSlowFakeAppImageExtractor(t, appImagePath)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := ExtractAppImage(ctx, appImagePath)
		done <- err
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want %v", err, context.Canceled)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ExtractAppImage did not return after cancellation")
	}
}

func TestLocateDesktopFileFallsBackToRecursiveSearch(t *testing.T) {
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

	got, err := LocateDesktopFile(root)
	if err != nil {
		t.Fatalf("LocateDesktopFile returned error: %v", err)
	}
	if got != preferred {
		t.Fatalf("LocateDesktopFile returned %q, want %q", got, preferred)
	}
}

func setupExtractionConfigForTest(t *testing.T, tmp string) {
	t.Helper()

	originalAimDir := config.AimDir
	originalTempDir := config.TempDir
	t.Cleanup(func() {
		config.AimDir = originalAimDir
		config.TempDir = originalTempDir
	})

	config.AimDir = filepath.Join(tmp, "aim")
	config.TempDir = filepath.Join(tmp, "cache", "tmp")
}

func writeFakeAppImageExtractor(t *testing.T, dst string) {
	t.Helper()

	writeFakeAppImageExtractorWithDesktop(t, dst, "0ad.desktop", "0 A.D.", "0.28.0", "0ad", "0ad.svg")
}

func writeFakeAppImageExtractorWithDesktop(t *testing.T, dst, desktopName, appName, version, iconName, iconFile string) {
	t.Helper()

	script := strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		"if [ \"${1:-}\" != \"--appimage-extract\" ]; then",
		"  echo \"unexpected args: $*\" >&2",
		"  exit 1",
		"fi",
		"mkdir -p squashfs-root/usr/share/applications",
		"cat > squashfs-root/usr/share/applications/" + desktopName + " <<'EOF'",
		"[Desktop Entry]",
		"Name=" + appName,
		"X-AppImage-Version=" + version,
		"Exec=AppRun %U",
		"Icon=" + iconName,
		"EOF",
		"ln -s usr/share/applications/" + desktopName + " squashfs-root/" + desktopName,
		"cat > squashfs-root/" + iconFile + " <<'EOF'",
		"<svg xmlns=\"http://www.w3.org/2000/svg\"/>",
		"EOF",
	}, "\n") + "\n"

	if err := os.WriteFile(dst, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake AppImage script: %v", err)
	}
}

func writeFakeAppImageExtractorWithoutDesktop(t *testing.T, dst string) {
	t.Helper()

	script := strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		"if [ \"${1:-}\" != \"--appimage-extract\" ]; then",
		"  echo \"unexpected args: $*\" >&2",
		"  exit 1",
		"fi",
		"mkdir -p squashfs-root",
		"cat > squashfs-root/0ad.svg <<'EOF'",
		"<svg xmlns=\"http://www.w3.org/2000/svg\"/>",
		"EOF",
	}, "\n") + "\n"

	if err := os.WriteFile(dst, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake AppImage script: %v", err)
	}
}

func writeSlowFakeAppImageExtractor(t *testing.T, dst string) {
	t.Helper()

	script := strings.Join([]string{
		"#!/bin/sh",
		"set -eu",
		"if [ \"${1:-}\" != \"--appimage-extract\" ]; then",
		"  echo \"unexpected args: $*\" >&2",
		"  exit 1",
		"fi",
		"sleep 5",
	}, "\n") + "\n"

	if err := os.WriteFile(dst, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write slow fake AppImage script: %v", err)
	}
}
