package integrate

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestValidateDesktopEntryDelegatesToConfiguredValidator(t *testing.T) {
	validator := &recordingDesktopEntryValidator{}
	service := Service{DesktopEntryValidator: validator}

	if err := service.ValidateDesktopEntry(context.Background(), "/tmp/app.desktop"); err != nil {
		t.Fatalf("ValidateDesktopEntry returned error: %v", err)
	}
	if validator.path != "/tmp/app.desktop" {
		t.Fatalf("validator path = %q, want /tmp/app.desktop", validator.path)
	}
}

func TestValidateDesktopEntryPropagatesValidatorError(t *testing.T) {
	service := Service{DesktopEntryValidator: &recordingDesktopEntryValidator{err: fmt.Errorf("invalid desktop entry")}}

	err := service.ValidateDesktopEntry(context.Background(), "/tmp/app.desktop")
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
