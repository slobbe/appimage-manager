package appimage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInspectExtractsToTemporaryRoot(t *testing.T) {
	appImagePath := filepath.Join(t.TempDir(), "test.AppImage")
	writeFakeExtractor(t, appImagePath, []string{
		"mkdir -p squashfs-root",
		"cat > squashfs-root/app.desktop <<'EOF'",
		"[Desktop Entry]",
		"Name=Test",
		"EOF",
	})

	extraction, cleanup, err := Inspect(context.Background(), appImagePath)
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	defer cleanup()

	if extraction.Dir == "" {
		t.Fatal("expected extraction dir to be set")
	}
	if _, err := os.Stat(filepath.Join(extraction.RootDir, "app.desktop")); err != nil {
		t.Fatalf("expected extracted desktop file: %v", err)
	}
}

func TestExtractReturnsCanceledPromptly(t *testing.T) {
	appImagePath := filepath.Join(t.TempDir(), "slow.AppImage")
	writeFakeExtractor(t, appImagePath, []string{
		"sleep 5",
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, _, err := Extract(ctx, appImagePath, filepath.Join(t.TempDir(), "extract"))
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
		t.Fatal("Extract did not return after cancellation")
	}
}

func TestExtractRejectsMissingSquashFSRoot(t *testing.T) {
	appImagePath := filepath.Join(t.TempDir(), "invalid.AppImage")
	writeFakeExtractor(t, appImagePath, []string{
		"mkdir -p other-root",
	})

	_, _, err := Extract(context.Background(), appImagePath, filepath.Join(t.TempDir(), "extract"))
	if err == nil {
		t.Fatal("expected error for missing squashfs-root")
	}
	if !strings.Contains(err.Error(), "squashfs-root not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeFakeExtractor(t *testing.T, dst string, body []string) {
	t.Helper()

	lines := append([]string{
		"#!/bin/sh",
		"set -eu",
		"if [ \"${1:-}\" != \"--appimage-extract\" ]; then",
		"  echo \"unexpected args: $*\" >&2",
		"  exit 1",
		"fi",
	}, body...)

	script := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(dst, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake AppImage script: %v", err)
	}
}
