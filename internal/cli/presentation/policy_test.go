package presentation

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestColorPolicy(t *testing.T) {
	writer := &bytes.Buffer{}
	tests := []struct {
		name      string
		mode      string
		noColor   bool
		tty       bool
		json      bool
		wantColor bool
	}{
		{name: "auto tty", mode: ColorAuto, tty: true, wantColor: true},
		{name: "auto redirected", mode: ColorAuto},
		{name: "NO_COLOR", mode: ColorAuto, noColor: true, tty: true},
		{name: "always overrides redirection", mode: ColorAlways, wantColor: true},
		{name: "always overrides NO_COLOR", mode: ColorAlways, noColor: true, wantColor: true},
		{name: "never overrides tty", mode: ColorNever, tty: true},
		{name: "JSON overrides always", mode: ColorAlways, tty: true, json: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := Policy{
				noColor: tt.noColor,
				isTTY: func(io.Writer) bool {
					return tt.tty
				},
			}
			got := policy.Success(writer, tt.mode, tt.json, "ok")
			if strings.Contains(got, "\033[") != tt.wantColor {
				t.Fatalf("Success() = %q, want color %t", got, tt.wantColor)
			}
		})
	}
}

func TestActivityRequiresHumanTTY(t *testing.T) {
	policy := Policy{isTTY: func(io.Writer) bool { return true }}
	if !policy.ActivityEnabled(&bytes.Buffer{}, false) {
		t.Fatal("ActivityEnabled() = false for human TTY")
	}
	if policy.ActivityEnabled(&bytes.Buffer{}, true) {
		t.Fatal("ActivityEnabled() = true for JSON")
	}

	policy.isTTY = func(io.Writer) bool { return false }
	if policy.ActivityEnabled(&bytes.Buffer{}, false) {
		t.Fatal("ActivityEnabled() = true for redirected output")
	}
}

func TestNewPolicyHonorsNOColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	policy := NewPolicy()
	policy.isTTY = func(io.Writer) bool { return true }
	if policy.ColorEnabled(&bytes.Buffer{}, ColorAuto, false) {
		t.Fatal("ColorEnabled() = true with NO_COLOR present")
	}
}

func TestValidateColorMode(t *testing.T) {
	for _, mode := range []string{ColorAuto, ColorAlways, ColorNever} {
		if err := ValidateColorMode(mode); err != nil {
			t.Fatalf("ValidateColorMode(%q) error = %v", mode, err)
		}
	}
	if err := ValidateColorMode("sometimes"); err == nil {
		t.Fatal("ValidateColorMode() error = nil for invalid value")
	}
}
