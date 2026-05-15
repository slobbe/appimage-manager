package integrate

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestValidateDesktopEntryDelegatesToConfiguredValidator(t *testing.T) {
	originalValidator := defaultDesktopEntryValidator
	t.Cleanup(func() {
		defaultDesktopEntryValidator = originalValidator
	})

	validator := &recordingDesktopEntryValidator{}
	SetDesktopEntryValidator(validator)

	if err := ValidateDesktopEntry(context.Background(), "/tmp/app.desktop"); err != nil {
		t.Fatalf("ValidateDesktopEntry returned error: %v", err)
	}
	if validator.path != "/tmp/app.desktop" {
		t.Fatalf("validator path = %q, want /tmp/app.desktop", validator.path)
	}
}

func TestValidateDesktopEntryPropagatesValidatorError(t *testing.T) {
	originalValidator := defaultDesktopEntryValidator
	t.Cleanup(func() {
		defaultDesktopEntryValidator = originalValidator
	})

	SetDesktopEntryValidator(&recordingDesktopEntryValidator{err: fmt.Errorf("invalid desktop entry")})

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

type recordingDesktopEntryValidator struct {
	path string
	err  error
}

func (validator *recordingDesktopEntryValidator) ValidateDesktopEntry(ctx context.Context, desktopPath string) error {
	validator.path = desktopPath
	return validator.err
}
