package app

import (
	"context"
	"errors"
)

// HostDiagnostics collects read-only host evidence; it does not launch an AppImage.
type HostDiagnostics interface {
	Inspect(context.Context) (HostDiagnosticsReport, error)
}

type DiagnosticStatus string

const (
	DiagnosticPass    DiagnosticStatus = "pass"
	DiagnosticWarning DiagnosticStatus = "warning"
	DiagnosticUnknown DiagnosticStatus = "unknown"
)

type DiagnosticSystem struct {
	ID           string `json:"id"`
	VersionID    string `json:"version_id"`
	VariantID    string `json:"variant_id"`
	PrettyName   string `json:"pretty_name"`
	Architecture string `json:"architecture"`
	Error        string `json:"error,omitempty"`
}

type DiagnosticProbe struct {
	State    DiagnosticStatus
	Evidence string
}

type HostDiagnosticsReport struct {
	System  DiagnosticSystem
	Library DiagnosticProbe
	Device  DiagnosticProbe
}

type DiagnoseRequest struct{}

type DiagnosticFinding struct {
	ID               string           `json:"id"`
	Status           DiagnosticStatus `json:"status"`
	Summary          string           `json:"summary"`
	Evidence         string           `json:"evidence"`
	Command          string           `json:"command,omitempty"`
	Guidance         string           `json:"guidance,omitempty"`
	DocumentationURL string           `json:"documentation_url,omitempty"`
}

type DiagnoseResult struct {
	System   DiagnosticSystem    `json:"system"`
	Findings []DiagnosticFinding `json:"findings"`
}

// ExitCode prioritizes incomplete checks over compatibility warnings.
func (r DiagnoseResult) ExitCode() int {
	code := 0
	for _, finding := range r.Findings {
		switch finding.Status {
		case DiagnosticPass:
		case DiagnosticWarning:
			code = 1
		default:
			return 2
		}
	}
	return code
}

const fuseDocumentation = "https://docs.appimage.org/user-guide/troubleshooting/fuse.html"

func (s *service) Diagnose(ctx context.Context, _ DiagnoseRequest) (DiagnoseResult, error) {
	if err := ctx.Err(); err != nil {
		return DiagnoseResult{}, err
	}
	if s.hostDiagnostics == nil {
		return DiagnoseResult{}, errors.New("host diagnostics adapter is required")
	}
	report, err := s.hostDiagnostics.Inspect(ctx)
	if err != nil {
		return DiagnoseResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return DiagnoseResult{}, err
	}
	result := DiagnoseResult{System: report.System}
	system := DiagnosticFinding{ID: "system", Status: DiagnosticPass, Summary: "Linux distribution identification", Evidence: report.System.ID}
	if report.System.Error != "" || report.System.ID == "" {
		system.Status = DiagnosticUnknown
		system.Evidence = report.System.Error
		if system.Evidence == "" {
			system.Evidence = "os-release does not identify the distribution"
		}
	}
	library := diagnosticFinding("fuse2_library", "Native FUSE 2 compatibility library", report.Library)
	if library.Status != DiagnosticPass {
		library.DocumentationURL = fuseDocumentation
		library.Guidance = "Some AppImages require libfuse.so.2, even when FUSE 3 is installed. A cache check does not cover custom library search paths or foreign architectures. Consult your distribution's documentation for FUSE 2 compatibility support."
		if library.Status == DiagnosticWarning && system.Status == DiagnosticPass {
			library.Command = fuseInstallCommand(report.System)
			if library.Command != "" {
				library.Guidance += " If needed, run the installation command yourself; aim does not install packages. Installing the library normally requires no reboot."
			}
		}
	}
	device := diagnosticFinding("fuse_device", "FUSE device access", report.Device)
	if device.Status != DiagnosticPass {
		device.DocumentationURL = fuseDocumentation
		device.Guidance = "A missing or inaccessible FUSE device is separate from a missing library. Check host or container restrictions with your distribution administrator; installing a library alone may not fix device access."
	}
	result.Findings = []DiagnosticFinding{system, library, device}
	return result, nil
}

func diagnosticFinding(id, summary string, probe DiagnosticProbe) DiagnosticFinding {
	status := probe.State
	if status != DiagnosticPass && status != DiagnosticWarning {
		status = DiagnosticUnknown
	}
	return DiagnosticFinding{ID: id, Summary: summary, Status: status, Evidence: probe.Evidence}
}

// Do not infer package compatibility from ID_LIKE or guess commands for editions.
func fuseInstallCommand(system DiagnosticSystem) string {
	if system.VariantID != "" {
		return ""
	}
	switch system.ID {
	case "ubuntu":
		switch system.VersionID {
		case "20.04", "22.04":
			return "sudo apt install libfuse2"
		case "24.04":
			return "sudo apt install libfuse2t64"
		}
	case "arch":
		return "sudo pacman -S fuse2"
	}
	return ""
}
