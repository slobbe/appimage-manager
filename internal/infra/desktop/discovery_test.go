package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscovererDiscoversRootDesktopEntry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "app.desktop")
	content := "[Desktop Entry]\nName=Example\n"
	writeFile(t, path, content)

	entry, err := Discoverer{}.Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if got, want := entry.Path, path; got != want {
		t.Fatalf("EntryFile.Path = %q, want %q", got, want)
	}
	if got := string(entry.Content); got != content {
		t.Fatalf("EntryFile.Content = %q, want %q", got, content)
	}
}

func TestDiscovererPrefersRootDesktopEntry(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	nested := filepath.Join(root, "usr", "share", "applications", "nested.desktop")
	rootEntry := filepath.Join(root, "app.desktop")
	writeFile(t, nested, "[Desktop Entry]\nName=Nested\n")
	writeFile(t, rootEntry, "[Desktop Entry]\nName=Root\n")

	entry, err := Discoverer{}.Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if got, want := entry.Path, rootEntry; got != want {
		t.Fatalf("EntryFile.Path = %q, want %q", got, want)
	}
}

func TestDiscovererFallsBackToApplicationsDirectory(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	misc := filepath.Join(root, "misc", "other.desktop")
	app := filepath.Join(root, "usr", "share", "applications", "app.desktop")
	writeFile(t, misc, "[Desktop Entry]\nName=Misc\n")
	writeFile(t, app, "[Desktop Entry]\nName=App\n")

	entry, err := Discoverer{}.Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if got, want := entry.Path, app; got != want {
		t.Fatalf("EntryFile.Path = %q, want %q", got, want)
	}
}

func TestDiscovererRequiresDesktopEntry(t *testing.T) {
	t.Parallel()

	_, err := Discoverer{}.Discover(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("Discover() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "no .desktop files found") {
		t.Fatalf("Discover() error = %q, want no .desktop files", err.Error())
	}
}

func TestDiscovererValidatesRoot(t *testing.T) {
	t.Parallel()

	_, err := Discoverer{}.Discover(context.Background(), "")
	if err == nil {
		t.Fatal("Discover() error = nil, want error")
	}
}

func TestDiscovererRespectsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Discoverer{}.Discover(ctx, t.TempDir())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Discover() error = %v, want context.Canceled", err)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %q: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}
