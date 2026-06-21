package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRefresherRunsAvailableRefreshCommands(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "refresh.log")
	for _, name := range []string{"update-desktop-database", "gtk-update-icon-cache", "xdg-desktop-menu", "xdg-icon-resource"} {
		writeRefreshCommand(t, binDir, name, logPath, "exit 0")
	}
	t.Setenv("PATH", binDir)

	refresher := Refresher{
		DesktopDir: "/home/user/.local/share/applications",
		IconDir:    "/home/user/.local/share/icons",
	}

	if err := refresher.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	want := []string{
		"update-desktop-database /home/user/.local/share/applications",
		"gtk-update-icon-cache -f -t /home/user/.local/share/icons/hicolor",
		"xdg-desktop-menu forceupdate",
		"xdg-icon-resource forceupdate",
	}
	if got := readRefreshLog(t, logPath); !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %#v, want %#v", got, want)
	}
}

func TestRefresherSkipsMissingRefreshCommands(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	refresher := Refresher{DesktopDir: "/applications", IconDir: "/icons"}
	if err := refresher.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
}

func TestRefresherSkipsPathSpecificCommandsWhenPathsMissing(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "refresh.log")
	for _, name := range []string{"update-desktop-database", "gtk-update-icon-cache", "xdg-desktop-menu", "xdg-icon-resource"} {
		writeRefreshCommand(t, binDir, name, logPath, "exit 0")
	}
	t.Setenv("PATH", binDir)

	if err := (Refresher{}).Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	want := []string{"xdg-desktop-menu forceupdate", "xdg-icon-resource forceupdate"}
	if got := readRefreshLog(t, logPath); !reflect.DeepEqual(got, want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestRefresherReturnsCommandFailures(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "refresh.log")
	writeRefreshCommand(t, binDir, "update-desktop-database", logPath, "exit 0")
	writeRefreshCommand(t, binDir, "gtk-update-icon-cache", logPath, "exit 7")
	writeRefreshCommand(t, binDir, "xdg-desktop-menu", logPath, "exit 0")
	writeRefreshCommand(t, binDir, "xdg-icon-resource", logPath, "exit 0")
	t.Setenv("PATH", binDir)

	err := (Refresher{DesktopDir: "/applications", IconDir: "/icons"}).Refresh(context.Background())
	if err == nil {
		t.Fatal("Refresh() error = nil, want command failure")
	}
	if !strings.Contains(err.Error(), "gtk-update-icon-cache") {
		t.Fatalf("Refresh() error = %q, want command name", err.Error())
	}
}

func TestRefresherRespectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Refresher{}.Refresh(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Refresh() error = %v, want context.Canceled", err)
	}
}

func writeRefreshCommand(t *testing.T, binDir string, name string, logPath string, trailer string) {
	t.Helper()
	script := "#!/bin/sh\nprintf '" + name + " %s\\n' \"$*\" >> " + shellQuote(logPath) + "\n" + trailer + "\n"
	path := filepath.Join(binDir, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write refresh command %s: %v", name, err)
	}
}

func readRefreshLog(t *testing.T, path string) []string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read refresh log: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(content)), "\n")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
