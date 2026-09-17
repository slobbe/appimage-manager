package selfupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
)

func TestCommandPassesPrereleaseFlag(t *testing.T) {
	service := &fakeService{
		selfUpdateResult: app.SelfUpdateResult{
			Applied: true,
			Update:  app.SelfUpdateCandidate{CurrentVersion: "0.17.0", NewVersion: "0.18.0-beta.1"},
		},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetArgs([]string{"--prerelease"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !service.selfUpdateReq.Prerelease {
		t.Fatal("SelfUpdateRequest.Prerelease = false, want true")
	}
	if !strings.Contains(stdout.String(), "Successfully updated aim to 0.18.0-beta.1!") {
		t.Fatalf("stdout = %q, want prerelease success", stdout.String())
	}
}

func TestCommandPrintsAlreadyUpToDate(t *testing.T) {
	service := &fakeService{
		selfUpdateResult: app.SelfUpdateResult{
			Applied: true,
			Update:  app.SelfUpdateCandidate{CurrentVersion: "0.18.0", NewVersion: "0.18.0"},
		},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(nil)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "aim is already up-to-date (0.18.0).") {
		t.Fatalf("stdout = %q, want up-to-date message", stdout.String())
	}
}

func TestCommandJSONStillRequiresConfirmation(t *testing.T) {
	service := &fakeService{
		selfUpdateResult: app.SelfUpdateResult{
			Update: app.SelfUpdateCandidate{CurrentVersion: "0.17.0", NewVersion: "0.18.0"},
		},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.JSON = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader("n\n"))

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	var payload struct {
		Applied bool `json:"applied"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	if payload.Applied {
		t.Fatal("payload.Applied = true, want false after rejecting JSON-mode confirmation")
	}
	if strings.Contains(stdout.String(), "(y/n)") {
		t.Fatalf("stdout = %q, want JSON only", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Update aim from 0.17.0 to 0.18.0? (y/n)") {
		t.Fatalf("stderr = %q, want confirmation prompt", stderr.String())
	}
}

func TestCommandYesAutoConfirmsInNonInteractiveMode(t *testing.T) {
	service := &fakeService{
		selfUpdateResult: app.SelfUpdateResult{
			Update: app.SelfUpdateCandidate{CurrentVersion: "0.17.0", NewVersion: "0.18.0"},
		},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.Yes = true
	rt.Config.NonInteractive = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !service.selfUpdateResult.Applied {
		t.Fatal("self-update not applied with --yes --non-interactive")
	}
	if strings.Contains(stdout.String(), "(y/n)") {
		t.Fatalf("stdout = %q, want no prompt", stdout.String())
	}
}

func TestCommandNonInteractiveFailsOnlyWhenUpdateNeedsConfirmation(t *testing.T) {
	service := &fakeService{
		selfUpdateResult: app.SelfUpdateResult{
			Update: app.SelfUpdateCandidate{CurrentVersion: "0.17.0", NewVersion: "0.18.0"},
		},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.JSON = true
	rt.Config.NonInteractive = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SilenceUsage = true

	err := cmd.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "confirmation required in non-interactive mode; rerun with --yes") {
		t.Fatalf("ExecuteContext() error = %v, want non-interactive confirmation error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no partial JSON", stdout.String())
	}
}

func TestCommandNonInteractiveAllowsAlreadyUpToDateResult(t *testing.T) {
	service := &fakeService{
		selfUpdateResult: app.SelfUpdateResult{
			Applied: true,
			Update:  app.SelfUpdateCandidate{CurrentVersion: "0.18.0", NewVersion: "0.18.0"},
		},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.NonInteractive = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "already up-to-date") {
		t.Fatalf("stdout = %q, want already up-to-date result", stdout.String())
	}
}

type fakeService struct {
	selfUpdateReq    app.SelfUpdateRequest
	selfUpdateResult app.SelfUpdateResult
	selfUpdateErr    error
}

var _ service = (*fakeService)(nil)

func (s *fakeService) SelfUpdate(ctx context.Context, req app.SelfUpdateRequest) (app.SelfUpdateResult, error) {
	s.selfUpdateReq = req
	if s.selfUpdateErr != nil {
		return app.SelfUpdateResult{}, s.selfUpdateErr
	}
	if s.selfUpdateResult.Update.CurrentVersion != s.selfUpdateResult.Update.NewVersion && req.Confirmation != nil {
		confirmed, err := req.Confirmation.ConfirmSelfUpdate(ctx, s.selfUpdateResult.Update)
		if err != nil {
			return app.SelfUpdateResult{}, err
		}
		s.selfUpdateResult.Applied = confirmed
	}
	return s.selfUpdateResult, nil
}
