package domain

import "testing"

func TestSlugify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{name: "handles empty", input: "", expect: ""},
		{name: "lowercases plain name", input: "Obsidian", expect: "obsidian"},
		{name: "replaces spaces", input: "Standard Notes", expect: "standard-notes"},
		{name: "replaces underscores", input: "standard_notes", expect: "standard-notes"},
		{name: "collapses repeated separators", input: "Standard   Notes___Desktop", expect: "standard-notes-desktop"},
		{name: "trims leading and trailing separators", input: "  Standard Notes  ", expect: "standard-notes"},
		{name: "drops punctuation", input: "T3 Code (Alpha)!", expect: "t3-code-alpha"},
		{name: "keeps numbers", input: "App 2.0", expect: "app-20"},
		{name: "keeps existing single dashes", input: "app-image-manager", expect: "app-image-manager"},
		{name: "collapses existing repeated dashes", input: "app---image", expect: "app-image"},
		{name: "drops slash without adding separator", input: "standardnotes/desktop", expect: "standardnotesdesktop"},
		{name: "keeps unicode letters", input: "Приложение 日本語", expect: "приложение-日本語"},
		{name: "removes combining marks", input: "Cafe\u0301", expect: "cafe"},
		{name: "drops symbols only", input: "!?@#$%^&*()", expect: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Slugify(tt.input); got != tt.expect {
				t.Fatalf("Slugify(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}
