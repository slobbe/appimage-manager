package integrate

import (
	"os"
	"strings"
	"testing"
)

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
