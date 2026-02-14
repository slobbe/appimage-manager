package core

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestValidateDesktopEntryValidatorMissingWarnsAndContinues(t *testing.T) {
	originalLookPath := desktopValidateLookPath
	originalCommand := desktopValidateCommandContext
	originalWarn := desktopValidateWarn
	t.Cleanup(func() {
		desktopValidateLookPath = originalLookPath
		desktopValidateCommandContext = originalCommand
		desktopValidateWarn = originalWarn
	})

	desktopValidateLookPath = func(string) (string, error) {
		return "", exec.ErrNotFound
	}
	desktopValidateCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		t.Fatal("command should not run when validator is missing")
		return exec.CommandContext(ctx, "sh", "-c", "exit 1")
	}

	warning := ""
	desktopValidateWarn = func(msg string) {
		warning = msg
	}

	err := ValidateDesktopEntry(context.Background(), "/tmp/app.desktop")
	if err != nil {
		t.Fatalf("ValidateDesktopEntry returned error: %v", err)
	}
	if !strings.Contains(warning, "skipping desktop entry validation") {
		t.Fatalf("warning = %q, want skipping message", warning)
	}
}

func TestValidateDesktopEntryValidatorPresentSuccess(t *testing.T) {
	originalLookPath := desktopValidateLookPath
	originalCommand := desktopValidateCommandContext
	originalWarn := desktopValidateWarn
	t.Cleanup(func() {
		desktopValidateLookPath = originalLookPath
		desktopValidateCommandContext = originalCommand
		desktopValidateWarn = originalWarn
	})

	desktopValidateLookPath = func(string) (string, error) {
		return "desktop-file-validate", nil
	}
	desktopValidateCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "exit 0")
	}

	calledWarn := false
	desktopValidateWarn = func(string) {
		calledWarn = true
	}

	err := ValidateDesktopEntry(context.Background(), "/tmp/app.desktop")
	if err != nil {
		t.Fatalf("ValidateDesktopEntry returned error: %v", err)
	}
	if calledWarn {
		t.Fatal("did not expect warning when validator succeeds")
	}
}

func TestValidateDesktopEntryValidatorPresentFailure(t *testing.T) {
	originalLookPath := desktopValidateLookPath
	originalCommand := desktopValidateCommandContext
	originalWarn := desktopValidateWarn
	t.Cleanup(func() {
		desktopValidateLookPath = originalLookPath
		desktopValidateCommandContext = originalCommand
		desktopValidateWarn = originalWarn
	})

	desktopValidateLookPath = func(string) (string, error) {
		return "desktop-file-validate", nil
	}
	desktopValidateCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "printf '%s' 'invalid desktop entry' >&2; exit 1")
	}
	desktopValidateWarn = func(string) {}

	err := ValidateDesktopEntry(context.Background(), "/tmp/app.desktop")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "invalid desktop entry") {
		t.Fatalf("error = %q, want validator output", err.Error())
	}
}

func TestValidateDesktopEntryRejectsEmptyPath(t *testing.T) {
	err := ValidateDesktopEntry(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}
