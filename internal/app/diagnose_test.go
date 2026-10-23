package app

import (
	"context"
	"errors"
	"testing"
)

type fakeHostDiagnostics struct {
	report HostDiagnosticsReport
	err    error
	calls  int
}

func (f *fakeHostDiagnostics) Inspect(context.Context) (HostDiagnosticsReport, error) {
	f.calls++
	return f.report, f.err
}

func TestDiagnose(t *testing.T) {
	for _, tt := range []struct {
		name            string
		library, device DiagnosticStatus
		system          DiagnosticSystem
		code            int
		command         string
	}{
		{"healthy", DiagnosticPass, DiagnosticPass, DiagnosticSystem{ID: "ubuntu", VersionID: "24.04"}, 0, ""},
		{"ubuntu missing library", DiagnosticWarning, DiagnosticPass, DiagnosticSystem{ID: "ubuntu", VersionID: "24.04"}, 1, "sudo apt install libfuse2t64"},
		{"older ubuntu", DiagnosticWarning, DiagnosticPass, DiagnosticSystem{ID: "ubuntu", VersionID: "22.04"}, 1, "sudo apt install libfuse2"},
		{"ubuntu 20", DiagnosticWarning, DiagnosticPass, DiagnosticSystem{ID: "ubuntu", VersionID: "20.04"}, 1, "sudo apt install libfuse2"},
		{"arch", DiagnosticWarning, DiagnosticPass, DiagnosticSystem{ID: "arch"}, 1, "sudo pacman -S fuse2"},
		{"unsupported distro", DiagnosticWarning, DiagnosticPass, DiagnosticSystem{ID: "other"}, 1, ""},
		{"unsupported release", DiagnosticWarning, DiagnosticPass, DiagnosticSystem{ID: "ubuntu", VersionID: "99.04"}, 1, ""},
		{"unsupported edition", DiagnosticWarning, DiagnosticPass, DiagnosticSystem{ID: "arch", VariantID: "immutable"}, 1, ""},
		{"device failure", DiagnosticPass, DiagnosticWarning, DiagnosticSystem{ID: "ubuntu", VersionID: "24.04"}, 1, ""},
		{"unknown library", DiagnosticUnknown, DiagnosticPass, DiagnosticSystem{ID: "ubuntu", VersionID: "24.04"}, 2, ""},
		{"unknown device beats warning", DiagnosticWarning, DiagnosticUnknown, DiagnosticSystem{ID: "arch"}, 2, "sudo pacman -S fuse2"},
		{"missing os release", DiagnosticWarning, DiagnosticPass, DiagnosticSystem{Error: "read: missing"}, 2, ""},
		{"missing distro id", DiagnosticPass, DiagnosticPass, DiagnosticSystem{}, 2, ""},
		{"invalid probe", "invalid", DiagnosticPass, DiagnosticSystem{ID: "arch"}, 2, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			probe := &fakeHostDiagnostics{report: HostDiagnosticsReport{System: tt.system, Library: DiagnosticProbe{State: tt.library, Evidence: "library evidence"}, Device: DiagnosticProbe{State: tt.device, Evidence: "device evidence"}}}
			s := &service{hostDiagnostics: probe}
			result, err := s.Diagnose(context.Background(), DiagnoseRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if probe.calls != 1 || result.System != tt.system || len(result.Findings) != 3 {
				t.Fatalf("unexpected result: %+v, calls %d", result, probe.calls)
			}
			if result.ExitCode() != tt.code {
				t.Fatalf("exit = %d, want %d", result.ExitCode(), tt.code)
			}
			library, device := result.Findings[1], result.Findings[2]
			if library.Command != tt.command || device.Command != "" {
				t.Fatalf("commands: library=%q device=%q", library.Command, device.Command)
			}
			if library.Evidence != "library evidence" || device.Evidence != "device evidence" {
				t.Fatal("lost original evidence")
			}
			if tt.library != DiagnosticPass && (library.Guidance == "" || library.DocumentationURL == "") {
				t.Fatal("missing fallback guidance")
			}
			if tt.device != DiagnosticPass && (device.Guidance == "" || device.DocumentationURL == "") {
				t.Fatal("missing device guidance")
			}
		})
	}
}

func TestDiagnoseErrors(t *testing.T) {
	failure := errors.New("probe failed")
	probe := &fakeHostDiagnostics{err: failure}
	s := &service{hostDiagnostics: probe}
	if _, err := s.Diagnose(context.Background(), DiagnoseRequest{}); !errors.Is(err, failure) {
		t.Fatalf("error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Diagnose(ctx, DiagnoseRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if probe.calls != 1 {
		t.Fatal("canceled request invoked probe")
	}
	if _, err := (&service{}).Diagnose(context.Background(), DiagnoseRequest{}); err == nil {
		t.Fatal("missing adapter accepted")
	}
}
