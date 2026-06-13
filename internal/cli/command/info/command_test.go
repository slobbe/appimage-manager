package info

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aim/internal/app"
	"aim/internal/cli/clienv"
)

func TestCommandPassesTargetAndPrintsTextInfo(t *testing.T) {
	service := &fakeService{
		infoResult: app.InfoResult{
			ID:       "example-app",
			Name:     "Example App",
			Version:  "1.2.3",
			ExecPath: "/apps/example-app.AppImage",
		},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"example-app"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if got, want := service.infoReq.Target, "example-app"; got != want {
		t.Fatalf("InfoRequest.Target = %q, want %q", got, want)
	}
	output := stdout.String()
	for _, want := range []string{
		"[example-app] Example App (v1.2.3)",
		"Exec path:        /apps/example-app.AppImage",
		"Source:           unknown",
		"Update source:    unknown",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want it to contain %q", output, want)
		}
	}
}

func TestCommandPrintsJSONInfo(t *testing.T) {
	service := &fakeService{
		infoResult: app.InfoResult{
			ID:       "example-app",
			Name:     "Example App",
			Version:  "1.2.3",
			ExecPath: "/apps/example-app.AppImage",
		},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.JSON = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"example-app"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	var payload struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Version  string `json:"version"`
		ExecPath string `json:"exec_path"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	if payload.ID != "example-app" || payload.Name != "Example App" || payload.Version != "1.2.3" || payload.ExecPath != "/apps/example-app.AppImage" {
		t.Fatalf("payload = %#v, want example app info", payload)
	}
	if got, want := service.infoReq.Target, "example-app"; got != want {
		t.Fatalf("InfoRequest.Target = %q, want %q", got, want)
	}
}

func TestCommandHelpDescribesIntegratedAppImages(t *testing.T) {
	cmd := NewCommand(clienv.New(&bytes.Buffer{}, &bytes.Buffer{}), &fakeService{})

	if got, want := cmd.Use, "info <app-id>"; got != want {
		t.Fatalf("Use = %q, want %q", got, want)
	}
	if strings.Contains(cmd.Long, "local AppImage file") || strings.Contains(cmd.Use, "path") {
		t.Fatalf("help promises local path support: Use=%q Long=%q", cmd.Use, cmd.Long)
	}
}

type fakeService struct {
	infoReq    app.InfoRequest
	infoResult app.InfoResult
	infoErr    error
}

var _ app.Service = (*fakeService)(nil)

func (s *fakeService) Add(ctx context.Context, req app.AddRequest) (app.AddResult, error) {
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
	s.infoReq = req
	if s.infoErr != nil {
		return app.InfoResult{}, s.infoErr
	}
	return s.infoResult, nil
}

func (s *fakeService) SelfUpdate(ctx context.Context, req app.SelfUpdateRequest) (app.SelfUpdateResult, error) {
	return app.SelfUpdateResult{}, nil
}

func (s *fakeService) Paths(ctx context.Context, req app.PathsRequest) (app.PathsResult, error) {
	return app.PathsResult{}, nil
}
