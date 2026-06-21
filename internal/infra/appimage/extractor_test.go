package appimage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractorExtractsAppImage(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	appImagePath := writeFakeAppImage(t, tmp, `#!/bin/sh
if [ "$1" != "--appimage-extract" ]; then
  echo "unexpected argument: $1" >&2
  exit 2
fi
mkdir squashfs-root
printf 'ok' > squashfs-root/app.desktop
`)
	destDir := filepath.Join(tmp, "extract")

	extraction, err := Extractor{}.Extract(context.Background(), appImagePath, destDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if got, want := extraction.RootDir, filepath.Join(destDir, extractedRootDirName); got != want {
		t.Fatalf("Extraction.RootDir = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(extraction.RootDir, "app.desktop")); err != nil {
		t.Fatalf("expected extracted desktop file: %v", err)
	}
}

func TestExtractorReturnsUpdateInformation(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	appImagePath := writeFakeAppImage(t, tmp, `#!/bin/sh
if [ "$1" = "--appimage-updateinformation" ]; then
  echo "gh-releases-zsync|owner|repo|latest|Example-*.AppImage.zsync"
  exit 0
fi
mkdir squashfs-root
`)
	destDir := filepath.Join(tmp, "extract")

	extraction, err := Extractor{}.Extract(context.Background(), appImagePath, destDir)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if got, want := extraction.UpdateInfo, "gh-releases-zsync|owner|repo|latest|Example-*.AppImage.zsync"; got != want {
		t.Fatalf("Extraction.UpdateInfo = %q, want %q", got, want)
	}
}

func TestExtractorMakesAppImageOwnerExecutable(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	appImagePath := writeFakeAppImage(t, tmp, `#!/bin/sh
mkdir squashfs-root
`)
	if err := os.Chmod(appImagePath, 0o600); err != nil {
		t.Fatalf("chmod fake appimage: %v", err)
	}

	_, err := Extractor{}.Extract(context.Background(), appImagePath, filepath.Join(tmp, "extract"))
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	info, err := os.Stat(appImagePath)
	if err != nil {
		t.Fatalf("stat fake appimage: %v", err)
	}
	if info.Mode()&0o100 == 0 {
		t.Fatalf("appimage mode = %v, want owner executable bit set", info.Mode())
	}
}

func TestExtractorReturnsCommandOutputOnFailure(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	appImagePath := writeFakeAppImage(t, tmp, `#!/bin/sh
echo "boom" >&2
exit 42
`)

	_, err := Extractor{}.Extract(context.Background(), appImagePath, filepath.Join(tmp, "extract"))
	if err == nil {
		t.Fatal("Extract() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("Extract() error = %q, want command output", err.Error())
	}
}

func TestExtractorRequiresSquashfsRoot(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	appImagePath := writeFakeAppImage(t, tmp, `#!/bin/sh
exit 0
`)

	_, err := Extractor{}.Extract(context.Background(), appImagePath, filepath.Join(tmp, "extract"))
	if err == nil {
		t.Fatal("Extract() error = nil, want error")
	}
	if !strings.Contains(err.Error(), extractedRootDirName) {
		t.Fatalf("Extract() error = %q, want missing extracted root", err.Error())
	}
}

func TestExtractorValidatesInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		appImagePath string
		destDir      string
	}{
		{name: "missing appimage path", appImagePath: "", destDir: "dest"},
		{name: "missing destination", appImagePath: "app.AppImage", destDir: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Extractor{}.Extract(context.Background(), tt.appImagePath, tt.destDir)
			if err == nil {
				t.Fatal("Extract() error = nil, want error")
			}
		})
	}
}

func TestExtractorRespectsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Extractor{}.Extract(ctx, "app.AppImage", "dest")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Extract() error = %v, want context.Canceled", err)
	}
}

func writeFakeAppImage(t *testing.T, dir string, script string) string {
	t.Helper()

	path := filepath.Join(dir, "fake.AppImage")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake appimage: %v", err)
	}

	return path
}
