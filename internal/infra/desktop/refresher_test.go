package desktop

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestRefresherRunsAvailableRefreshCommands(t *testing.T) {
	t.Parallel()

	var calls []refreshCall
	refresher := Refresher{
		DesktopDir: "/home/user/.local/share/applications",
		IconDir:    "/home/user/.local/share/icons",
		LookPath: func(name string) (string, error) {
			return "/usr/bin/" + name, nil
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			calls = append(calls, refreshCall{name: name, args: append([]string(nil), args...)})
			return nil
		},
	}

	if err := refresher.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	want := []refreshCall{
		{name: "/usr/bin/update-desktop-database", args: []string{"/home/user/.local/share/applications"}},
		{name: "/usr/bin/gtk-update-icon-cache", args: []string{"-f", "-t", "/home/user/.local/share/icons/hicolor"}},
		{name: "/usr/bin/xdg-desktop-menu", args: []string{"forceupdate"}},
		{name: "/usr/bin/xdg-icon-resource", args: []string{"forceupdate"}},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %#v, want %#v", calls, want)
	}
}

func TestRefresherSkipsMissingRefreshCommands(t *testing.T) {
	t.Parallel()

	refresher := Refresher{
		DesktopDir: "/applications",
		IconDir:    "/icons",
		LookPath: func(name string) (string, error) {
			return "", exec.ErrNotFound
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			t.Fatalf("Run() called for missing command %q", name)
			return nil
		},
	}

	if err := refresher.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
}

func TestRefresherSkipsPathSpecificCommandsWhenPathsMissing(t *testing.T) {
	t.Parallel()

	var commands []string
	refresher := Refresher{
		LookPath: func(name string) (string, error) {
			return name, nil
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			commands = append(commands, name)
			return nil
		},
	}

	if err := refresher.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}

	want := []string{"xdg-desktop-menu", "xdg-icon-resource"}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
}

func TestRefresherReturnsCommandFailures(t *testing.T) {
	t.Parallel()

	failure := errors.New("boom")
	refresher := Refresher{
		DesktopDir: "/applications",
		IconDir:    "/icons",
		LookPath: func(name string) (string, error) {
			return name, nil
		},
		Run: func(ctx context.Context, name string, args ...string) error {
			if name == "gtk-update-icon-cache" {
				return failure
			}
			return nil
		},
	}

	err := refresher.Refresh(context.Background())
	if !errors.Is(err, failure) {
		t.Fatalf("Refresh() error = %v, want %v", err, failure)
	}
	if !strings.Contains(err.Error(), "gtk-update-icon-cache") {
		t.Fatalf("Refresh() error = %q, want command name", err.Error())
	}
}

func TestRefresherRespectsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Refresher{}.Refresh(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Refresh() error = %v, want context.Canceled", err)
	}
}

type refreshCall struct {
	name string
	args []string
}
