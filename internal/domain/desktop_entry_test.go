package domain

import (
	"strings"
	"testing"
)

func TestParseDesktopEntry(t *testing.T) {
	t.Parallel()

	entry, err := ParseDesktopEntry([]byte(`
# comment before group
[Desktop Entry]
Type=Application
Name=Example App
Exec=/opt/example/example.AppImage --flag
Icon=example-icon
Version=v1.2.3-beta.1
Comment=An example app

[Desktop Action NewWindow]
Name=New Window
Exec=ignored
`))
	if err != nil {
		t.Fatalf("ParseDesktopEntry() error = %v", err)
	}

	if got, want := entry.Name, "Example App"; got != want {
		t.Fatalf("DesktopEntry.Name = %q, want %q", got, want)
	}
	if got, want := entry.Exec, "/opt/example/example.AppImage --flag"; got != want {
		t.Fatalf("DesktopEntry.Exec = %q, want %q", got, want)
	}
	if got, want := entry.Icon, "example-icon"; got != want {
		t.Fatalf("DesktopEntry.Icon = %q, want %q", got, want)
	}
	if got, want := entry.Version.String(), "1.2.3-beta.1"; got != want {
		t.Fatalf("DesktopEntry.Version = %q, want %q", got, want)
	}
	if got, want := entry.Fields["Type"], "Application"; got != want {
		t.Fatalf("DesktopEntry.Fields[Type] = %q, want %q", got, want)
	}
	if got, want := entry.Fields["Comment"], "An example app"; got != want {
		t.Fatalf("DesktopEntry.Fields[Comment] = %q, want %q", got, want)
	}
	if got, want := entry.Fields["Name"], "Example App"; got != want {
		t.Fatalf("DesktopEntry.Fields[Name] = %q, want %q", got, want)
	}
	if got := entry.Fields["Desktop Action NewWindow"]; got != "" {
		t.Fatalf("DesktopEntry.Fields contains action group data: %q", got)
	}
}

func TestParseDesktopEntryPrefersAppImageNameAndVersion(t *testing.T) {
	t.Parallel()

	entry, err := ParseDesktopEntry([]byte(`
[Desktop Entry]
Name=Generic Name
Version=0.1.0
X-AppImage-Name=AppImage Name
X-AppImage-Version=1.0.0-beta.2
`))
	if err != nil {
		t.Fatalf("ParseDesktopEntry() error = %v", err)
	}

	if got, want := entry.Name, "AppImage Name"; got != want {
		t.Fatalf("DesktopEntry.Name = %q, want %q", got, want)
	}
	if got, want := entry.Version.String(), "1.0.0-beta.2"; got != want {
		t.Fatalf("DesktopEntry.Version = %q, want %q", got, want)
	}
	if got, want := entry.Fields["Name"], "Generic Name"; got != want {
		t.Fatalf("DesktopEntry.Fields[Name] = %q, want %q", got, want)
	}
	if got, want := entry.Fields["X-AppImage-Name"], "AppImage Name"; got != want {
		t.Fatalf("DesktopEntry.Fields[X-AppImage-Name] = %q, want %q", got, want)
	}
}

func TestParseDesktopEntryFallsBackToStandardNameAndVersion(t *testing.T) {
	t.Parallel()

	entry, err := ParseDesktopEntry([]byte(`
[Desktop Entry]
Name=Generic Name
Version=0.1.0
`))
	if err != nil {
		t.Fatalf("ParseDesktopEntry() error = %v", err)
	}

	if got, want := entry.Name, "Generic Name"; got != want {
		t.Fatalf("DesktopEntry.Name = %q, want %q", got, want)
	}
	if got, want := entry.Version.String(), "0.1.0"; got != want {
		t.Fatalf("DesktopEntry.Version = %q, want %q", got, want)
	}
}

func TestParseDesktopEntryTrimsKeysAndValues(t *testing.T) {
	t.Parallel()

	entry, err := ParseDesktopEntry([]byte("[Desktop Entry]\n Name = Example \n Exec = /bin/example \n Icon = example \n"))
	if err != nil {
		t.Fatalf("ParseDesktopEntry() error = %v", err)
	}

	if got, want := entry.Name, "Example"; got != want {
		t.Fatalf("DesktopEntry.Name = %q, want %q", got, want)
	}
	if got, want := entry.Exec, "/bin/example"; got != want {
		t.Fatalf("DesktopEntry.Exec = %q, want %q", got, want)
	}
	if got, want := entry.Icon, "example"; got != want {
		t.Fatalf("DesktopEntry.Icon = %q, want %q", got, want)
	}
}

func TestParseDesktopEntryAllowsMissingOptionalFields(t *testing.T) {
	t.Parallel()

	entry, err := ParseDesktopEntry([]byte("[Desktop Entry]\nName=Example\n"))
	if err != nil {
		t.Fatalf("ParseDesktopEntry() error = %v", err)
	}

	if got, want := entry.Name, "Example"; got != want {
		t.Fatalf("DesktopEntry.Name = %q, want %q", got, want)
	}
	if entry.Version.String() != "" {
		t.Fatalf("DesktopEntry.Version = %q, want empty", entry.Version.String())
	}
}

func TestDesktopEntryWithExecAndIconPreservesRawLinesGroupsAndOrdering(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		"# file comment",
		"[Desktop Entry]",
		"Type=Application",
		"# keep this comment",
		"Name=Example",
		"Exec=/tmp/old.AppImage --old-flag",
		"Icon=old-icon",
		"X-Custom=value",
		"",
		"[Desktop Action NewWindow]",
		"Name=New Window",
		"Exec=/tmp/old.AppImage --new-window",
		"",
	}, "\n")

	entry, err := ParseDesktopEntry([]byte(input))
	if err != nil {
		t.Fatalf("ParseDesktopEntry() error = %v", err)
	}

	updated := entry.WithExec("/apps/example.AppImage").WithIcon("/icons/example.png")

	if got, want := updated.Exec, "/apps/example.AppImage"; got != want {
		t.Fatalf("DesktopEntry.Exec = %q, want %q", got, want)
	}
	if got, want := updated.Icon, "/icons/example.png"; got != want {
		t.Fatalf("DesktopEntry.Icon = %q, want %q", got, want)
	}
	if got, want := entry.Exec, "/tmp/old.AppImage --old-flag"; got != want {
		t.Fatalf("original DesktopEntry.Exec = %q, want %q", got, want)
	}

	got := string(updated.Bytes())
	want := strings.Join([]string{
		"# file comment",
		"[Desktop Entry]",
		"Type=Application",
		"# keep this comment",
		"Name=Example",
		"Exec=/apps/example.AppImage",
		"Icon=/icons/example.png",
		"X-Custom=value",
		"",
		"[Desktop Action NewWindow]",
		"Name=New Window",
		"Exec=/apps/example.AppImage --new-window",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("updated.Bytes() =\n%q\nwant\n%q", got, want)
	}
}

func TestDesktopEntryWithExecReplacesActionCommandToken(t *testing.T) {
	t.Parallel()

	entry, err := ParseDesktopEntry([]byte(strings.Join([]string{
		"[Desktop Entry]",
		"Name=Example",
		"Exec=old-main",
		"",
		"[Desktop Action NewWindow]",
		"Name=New Window",
		"Exec=old-action --new-window",
		"",
	}, "\n")))
	if err != nil {
		t.Fatalf("ParseDesktopEntry() error = %v", err)
	}

	updated := entry.WithExec("/apps/example.AppImage")

	got := string(updated.Bytes())
	want := strings.Join([]string{
		"[Desktop Entry]",
		"Name=Example",
		"Exec=/apps/example.AppImage",
		"",
		"[Desktop Action NewWindow]",
		"Name=New Window",
		"Exec=/apps/example.AppImage --new-window",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("updated.Bytes() =\n%q\nwant\n%q", got, want)
	}
}

func TestDesktopEntryWithExecAndIconAddsMissingFieldsBeforeNextGroup(t *testing.T) {
	t.Parallel()

	entry, err := ParseDesktopEntry([]byte(strings.Join([]string{
		"[Desktop Entry]",
		"Name=Example",
		"",
		"[Desktop Action NewWindow]",
		"Name=New Window",
		"",
	}, "\n")))
	if err != nil {
		t.Fatalf("ParseDesktopEntry() error = %v", err)
	}

	updated := entry.WithExec("/apps/example.AppImage").WithIcon("example")

	got := string(updated.Bytes())
	want := strings.Join([]string{
		"[Desktop Entry]",
		"Name=Example",
		"",
		"Exec=/apps/example.AppImage",
		"Icon=example",
		"[Desktop Action NewWindow]",
		"Name=New Window",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("updated.Bytes() =\n%q\nwant\n%q", got, want)
	}
}

func TestDesktopEntryBytesFromManualEntry(t *testing.T) {
	t.Parallel()

	entry := DesktopEntry{
		Fields: map[string]string{
			"Type": "Application",
			"Name": "Example",
			"Exec": "/apps/example.AppImage",
			"Icon": "example",
		},
	}

	got := string(entry.Bytes())
	want := strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=Example",
		"Exec=/apps/example.AppImage",
		"Icon=example",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("entry.Bytes() =\n%q\nwant\n%q", got, want)
	}
}

func TestParseDesktopEntryRequiresDesktopEntryGroup(t *testing.T) {
	t.Parallel()

	_, err := ParseDesktopEntry([]byte("[Other]\nName=Example\n"))
	if err == nil {
		t.Fatal("ParseDesktopEntry() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing [Desktop Entry] group") {
		t.Fatalf("ParseDesktopEntry() error = %q, want missing group", err.Error())
	}
}

func TestParseDesktopEntryRejectsMalformedLine(t *testing.T) {
	t.Parallel()

	_, err := ParseDesktopEntry([]byte("[Desktop Entry]\nName\n"))
	if err == nil {
		t.Fatal("ParseDesktopEntry() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing '='") {
		t.Fatalf("ParseDesktopEntry() error = %q, want missing '='", err.Error())
	}
}
