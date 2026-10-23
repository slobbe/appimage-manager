package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
)

func TestRootGlobalConfirmationFlagsReachMutationCommands(t *testing.T) {
	service := &rootTestService{
		update: app.UpdateCandidate{
			ID:             "example-app",
			CurrentVersion: "1.0.0",
			NewVersion:     "2.0.0",
		},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	cmd := NewRootCommand(rt, service, "test")
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(failingReader{})
	cmd.SetArgs([]string{"--yes", "--non-interactive", "update", "example-app"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !rt.Config.Yes || !rt.Config.NonInteractive {
		t.Fatalf("runtime config = %#v, want yes and non-interactive", rt.Config)
	}
	if !service.confirmed {
		t.Fatal("update confirmation = false, want true")
	}
}

func TestRootRejectsInvalidColorMode(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewRootCommand(clienv.New(stdout, stderr), &rootTestService{}, "test")
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"--color=sometimes", "update"})

	err := cmd.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "must be auto, always, or never") {
		t.Fatalf("ExecuteContext() error = %v, want invalid color mode", err)
	}
}

type rootTestService struct {
	app.Service
	update       app.UpdateCandidate
	confirmed    bool
	diagnosis    app.DiagnoseResult
	diagnosisErr error
}

func (s *rootTestService) Update(ctx context.Context, req app.UpdateRequest) (app.UpdateResult, error) {
	confirmed, err := req.Confirmation.ConfirmUpdates(ctx, []app.UpdateCandidate{s.update})
	if err != nil {
		return app.UpdateResult{}, err
	}
	s.confirmed = confirmed
	return app.UpdateResult{Applied: confirmed, Updates: []app.UpdateCandidate{s.update}}, nil
}

func (s *rootTestService) Diagnose(context.Context, app.DiagnoseRequest) (app.DiagnoseResult, error) {
	return s.diagnosis, s.diagnosisErr
}

func TestExecuteDiagnoseExitCodes(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status app.DiagnosticStatus
		err    error
		code   int
	}{
		{"healthy", app.DiagnosticPass, nil, 0},
		{"warning", app.DiagnosticWarning, nil, 1},
		{"unknown", app.DiagnosticUnknown, nil, 2},
		{"failed", "", errors.New("probe failed"), 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := &rootTestService{diagnosis: app.DiagnoseResult{Findings: []app.DiagnosticFinding{{ID: "test", Status: tt.status}}}, diagnosisErr: tt.err}
			var out, errOut bytes.Buffer
			code := Execute(context.Background(), []string{"--json", "--yes", "--non-interactive", "diagnose"}, &out, &errOut, svc, "test")
			if code != tt.code {
				t.Fatalf("code = %d, want %d", code, tt.code)
			}
			if tt.err == nil {
				if !json.Valid(out.Bytes()) || errOut.Len() != 0 {
					t.Fatalf("stdout=%q stderr=%q", out.String(), errOut.String())
				}
			} else if out.Len() != 0 || errOut.String() != "probe failed\n" {
				t.Fatalf("stdout=%q stderr=%q", out.String(), errOut.String())
			}
		})
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
