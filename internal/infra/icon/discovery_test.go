package icon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscovererUsesAbsoluteIconPathInsideRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	iconPath := filepath.Join(root, "usr", "share", "icons", "app.png")
	writeIcon(t, iconPath)

	file, err := NewDiscoverer().Discover(context.Background(), root, iconPath)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got, want := file.Path, iconPath; got != want {
		t.Fatalf("File.Path = %q, want %q", got, want)
	}
}

func TestDiscovererRejectsAbsoluteIconPathOutsideRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "app.png")
	writeIcon(t, outside)

	_, err := NewDiscoverer().Discover(context.Background(), root, outside)
	if err == nil {
		t.Fatal("Discover() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "outside extracted root") {
		t.Fatalf("Discover() error = %q, want outside root", err.Error())
	}
}

func TestDiscovererFindsIconByNameWithoutExtension(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	iconPath := filepath.Join(root, "usr", "share", "icons", "hicolor", "128x128", "apps", "example.png")
	writeIcon(t, iconPath)

	file, err := NewDiscoverer().Discover(context.Background(), root, "example")
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got, want := file.Path, iconPath; got != want {
		t.Fatalf("File.Path = %q, want %q", got, want)
	}
}

func TestDiscovererFindsIconByNameWithExtension(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	iconPath := filepath.Join(root, "usr", "share", "pixmaps", "example.svg")
	writeIcon(t, iconPath)

	file, err := NewDiscoverer().Discover(context.Background(), root, "example.svg")
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got, want := file.Path, iconPath; got != want {
		t.Fatalf("File.Path = %q, want %q", got, want)
	}
}

func TestDiscovererFindsIconByRelativePath(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	iconPath := filepath.Join(root, "custom", "icons", "example.png")
	writeIcon(t, iconPath)

	file, err := NewDiscoverer().Discover(context.Background(), root, "custom/icons/example.png")
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got, want := file.Path, iconPath; got != want {
		t.Fatalf("File.Path = %q, want %q", got, want)
	}
}

func TestDiscovererPrefersLargerThemeIcon(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	small := filepath.Join(root, "usr", "share", "icons", "hicolor", "64x64", "apps", "example.png")
	large := filepath.Join(root, "usr", "share", "icons", "hicolor", "512x512", "apps", "example.png")
	writeIcon(t, small)
	writeIcon(t, large)

	file, err := NewDiscoverer().Discover(context.Background(), root, "example")
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got, want := file.Path, large; got != want {
		t.Fatalf("File.Path = %q, want %q", got, want)
	}
}

func TestDiscovererUsesDirIconAsFallback(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dirIcon := filepath.Join(root, ".DirIcon")
	writeIcon(t, dirIcon)

	file, err := NewDiscoverer().Discover(context.Background(), root, "")
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got, want := file.Path, dirIcon; got != want {
		t.Fatalf("File.Path = %q, want %q", got, want)
	}
}

func TestDiscovererPrefersNamedIconOverDirIcon(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dirIcon := filepath.Join(root, ".DirIcon")
	named := filepath.Join(root, "usr", "share", "icons", "hicolor", "256x256", "apps", "example.png")
	writeIcon(t, dirIcon)
	writeIcon(t, named)

	file, err := NewDiscoverer().Discover(context.Background(), root, "example")
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if got, want := file.Path, named; got != want {
		t.Fatalf("File.Path = %q, want %q", got, want)
	}
}

func TestDiscovererRequiresSupportedIcon(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeIcon(t, filepath.Join(root, "example.txt"))

	_, err := NewDiscoverer().Discover(context.Background(), root, "")
	if err == nil {
		t.Fatal("Discover() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "no supported icon files found") {
		t.Fatalf("Discover() error = %q, want no supported icon files", err.Error())
	}
}

func TestDiscovererValidatesRoot(t *testing.T) {
	t.Parallel()

	_, err := NewDiscoverer().Discover(context.Background(), "", "example")
	if err == nil {
		t.Fatal("Discover() error = nil, want error")
	}
}

func TestDiscovererRespectsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewDiscoverer().Discover(ctx, t.TempDir(), "example")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover() error = %v, want context.Canceled", err)
	}
}

func writeIcon(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("icon"), 0o644); err != nil {
		t.Fatalf("write icon %q: %v", path, err)
	}
}
