package add

import (
	"bytes"
	"context"
	"testing"

	"aim/internal/app"
	"aim/internal/cli/clienv"
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
	addReq app.AddRequest
}

var _ app.Service = (*fakeService)(nil)

func (s *fakeService) Add(ctx context.Context, req app.AddRequest) (app.AddResult, error) {
	s.addReq = req
	return app.AddResult{}, nil
}

func (s *fakeService) Remove(ctx context.Context, req app.RemoveRequest) error {
	return nil
}

func (s *fakeService) Update(ctx context.Context, req app.UpdateRequest) (app.UpdateResult, error) {
	return app.UpdateResult{}, nil
}

func (s *fakeService) SetUpdateSource(ctx context.Context, req app.SetUpdateSourceRequest) (app.SetUpdateSourceResult, error) {
	return app.SetUpdateSourceResult{}, nil
}

func (s *fakeService) UnsetUpdateSource(ctx context.Context, req app.UnsetUpdateSourceRequest) error {
	return nil
}

func (s *fakeService) List(ctx context.Context, req app.ListRequest) (app.ListResult, error) {
	return app.ListResult{}, nil
}

func (s *fakeService) Info(ctx context.Context, req app.InfoRequest) (app.InfoResult, error) {
	return app.InfoResult{}, nil
}

func (s *fakeService) SelfUpdate(ctx context.Context, req app.SelfUpdateRequest) (app.SelfUpdateResult, error) {
	return app.SelfUpdateResult{}, nil
}

func (s *fakeService) Paths(ctx context.Context, req app.PathsRequest) (app.PathsResult, error) {
	return app.PathsResult{}, nil
}
