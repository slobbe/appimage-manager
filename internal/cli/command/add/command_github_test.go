package add

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
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

func TestCommandPassesGitHubAssetFilters(t *testing.T) {
	for _, asset := range []string{"", "Example-*.AppImage"} {
		t.Run(asset, func(t *testing.T) {
			service := &fakeService{}
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			cmd := NewCommand(clienv.New(stdout, stderr), service)
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)
			args := []string{"--github", "owner/repo", "--exclude", "*arm*", "--exclude", "*debug,test*", "--prefer", "*portable*", "--prefer", "*one,two*"}
			if asset != "" {
				args = append(args, "--asset", asset)
			}
			cmd.SetArgs(args)
			if err := cmd.ExecuteContext(context.Background()); err != nil {
				t.Fatalf("ExecuteContext() error = %v", err)
			}
			if got, want := service.addReq.AssetExcludePatterns, []string{"*arm*", "*debug,test*"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("AssetExcludePatterns = %q, want %q", got, want)
			}
			if got, want := service.addReq.AssetPreferPatterns, []string{"*portable*", "*one,two*"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("AssetPreferPatterns = %q, want %q", got, want)
			}
		})
	}
}

func TestCommandRejectsInvalidAssetFilters(t *testing.T) {
	for _, flag := range []string{"--exclude", "--prefer"} {
		for _, tc := range []struct {
			args []string
			want string
		}{
			{[]string{flag, "*debug*"}, flag + " requires --github"},
			{[]string{"Example.AppImage", flag, "*debug*"}, flag + " requires --github"},
			{[]string{"--github", "owner/repo", flag, ""}, flag + " pattern must not be empty"},
			{[]string{"--github", "owner/repo", flag, "*valid*", flag, " \t "}, flag + " pattern must not be empty"},
		} {
			t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
				service := &fakeService{}
				stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
				cmd := NewCommand(clienv.New(stdout, stderr), service)
				cmd.SetOut(stdout)
				cmd.SetErr(stderr)
				cmd.SetArgs(tc.args)
				if err := cmd.ExecuteContext(context.Background()); err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("ExecuteContext() error = %v, want %q", err, tc.want)
				}
				if !reflect.DeepEqual(service.addReq, app.AddRequest{}) {
					t.Fatal("service.Add was called for invalid flags")
				}
			})
		}
	}
}

type fakeService struct {
	addReq app.AddRequest
}

var _ service = (*fakeService)(nil)

func (s *fakeService) Add(ctx context.Context, req app.AddRequest) (app.AddResult, error) {
	s.addReq = req
	return app.AddResult{}, nil
}
