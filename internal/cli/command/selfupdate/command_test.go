package selfupdate

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"aim/internal/app"
	"aim/internal/cli/clienv"
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

type fakeService struct {
	selfUpdateReq    app.SelfUpdateRequest
	selfUpdateResult app.SelfUpdateResult
	selfUpdateErr    error
}

var _ app.SelfUpdateRunner = (*fakeService)(nil)

func (s *fakeService) SelfUpdate(ctx context.Context, req app.SelfUpdateRequest) (app.SelfUpdateResult, error) {
	s.selfUpdateReq = req
	if s.selfUpdateErr != nil {
		return app.SelfUpdateResult{}, s.selfUpdateErr
	}
	return s.selfUpdateResult, nil
}
