package add

import (
	"bytes"
	"context"
	"testing"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
	"github.com/slobbe/appimage-manager/internal/cli/command/commandtest"
)

func TestCommandPassesGitHubAssetPattern(t *testing.T) {
	service := &fakeService{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"--github", "owner/repo", "--asset", "Example-*.AppImage"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if got, want := service.addReq.GitHubRepo, "owner/repo"; got != want {
		t.Fatalf("AddRequest.GitHubRepo = %q, want %q", got, want)
	}
	if got, want := service.addReq.AssetPattern, "Example-*.AppImage"; got != want {
		t.Fatalf("AddRequest.AssetPattern = %q, want %q", got, want)
	}
	if service.addReq.Path != "" {
		t.Fatalf("AddRequest.Path = %q, want empty", service.addReq.Path)
	}
}

func TestCommandRejectsAssetWithoutGitHub(t *testing.T) {
	service := &fakeService{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"--asset", "Example-*.AppImage"})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("ExecuteContext() error = nil, want --asset validation error")
	}
}

type fakeService struct {
	commandtest.Service
	addReq app.AddRequest
}

var _ app.Service = (*fakeService)(nil)

func (s *fakeService) Add(ctx context.Context, req app.AddRequest) (app.AddResult, error) {
	s.addReq = req
	return app.AddResult{}, nil
}
