package domain

import "testing"

func TestSanitizeDesktopStem(t *testing.T) {
	got := SanitizeDesktopStem("  My App__Beta!!.desktop  ")
	if got != "My-App_Beta.desktop" {
		t.Fatalf("SanitizeDesktopStem = %q", got)
	}
}

func TestDesktopStemFromPath(t *testing.T) {
	if got := DesktopStemFromPath("/tmp/My App.desktop"); got != "My App" {
		t.Fatalf("DesktopStemFromPath = %q, want My App", got)
	}
}
