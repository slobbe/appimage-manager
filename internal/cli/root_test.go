package cli

import (
	"bytes"
	"context"
	"io"
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

type rootTestService struct {
	app.Service
	update    app.UpdateCandidate
	confirmed bool
}

func (s *rootTestService) Update(ctx context.Context, req app.UpdateRequest) (app.UpdateResult, error) {
	confirmed, err := req.Confirmation.ConfirmUpdates(ctx, []app.UpdateCandidate{s.update})
	if err != nil {
		return app.UpdateResult{}, err
	}
	s.confirmed = confirmed
	return app.UpdateResult{Applied: confirmed, Updates: []app.UpdateCandidate{s.update}}, nil
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
