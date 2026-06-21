package info

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
)

func TestCommandRequiresExactlyOneArg(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "none", args: nil},
		{name: "too many", args: []string{"one", "two"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			service := &fakeService{}
			cmd := NewCommand(clienv.New(stdout, stderr), service)
			cmd.SetOut(stdout)
			cmd.SetErr(stderr)
			cmd.SetArgs(tc.args)

			if err := cmd.ExecuteContext(context.Background()); err == nil {
				t.Fatal("ExecuteContext() error = nil, want arg validation error")
			}
			if service.infoCalled {
				t.Fatal("service.Info was called for invalid args")
			}
		})
	}
}

func TestCommandPassesTargetAndPrintsTextInfo(t *testing.T) {
	service := &fakeService{
		infoResult: app.InfoResult{
			ID:         "example-app",
			Name:       "Example App",
			Version:    "1.2.3",
			ExecPath:   "/apps/example-app.AppImage",
			Installed:  true,
			TargetKind: "installed",
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
		"Status:           installed",
		"Exec path:        /apps/example-app.AppImage",
		"Source:           unknown",
		"Update source:    unknown",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want it to contain %q", output, want)
		}
	}
}

func TestCommandPrintsPreservedNonGitHubUpdateSourceStatus(t *testing.T) {
	result := app.InfoResult{
		Name:       "Example App",
		Version:    "1.2.3",
		ExecPath:   "/apps/example-app.AppImage",
		TargetKind: "installed",
	}
	result.UpdateSource.Embedded = true
	result.UpdateSource.Kind = "zsync"
	result.UpdateSource.URL = "https://example.test/App.AppImage.zsync"

	service := &fakeService{infoResult: result}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"example-app"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"Update source:    zsync",
		"Zsync URL:        https://example.test/App.AppImage.zsync",
		"Update support:   preserved; updates not applied by aim yet",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want it to contain %q", output, want)
		}
	}
}

func TestCommandPrintsJSONInfo(t *testing.T) {
	service := &fakeService{
		infoResult: app.InfoResult{
			ID:         "example-app",
			Name:       "Example App",
			Version:    "1.2.3",
			ExecPath:   "/apps/example-app.AppImage",
			Installed:  true,
			TargetKind: "installed",
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
		ID         string `json:"id"`
		Name       string `json:"name"`
		Version    string `json:"version"`
		ExecPath   string `json:"exec_path"`
		Installed  bool   `json:"installed"`
		TargetKind string `json:"target_kind"`
		Source     struct {
			Kind string `json:"kind"`
		} `json:"source"`
		UpdateSource struct {
			Kind string `json:"kind"`
		} `json:"update_source"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	if payload.ID != "example-app" || payload.Name != "Example App" || payload.Version != "1.2.3" || payload.ExecPath != "/apps/example-app.AppImage" || !payload.Installed || payload.TargetKind != "installed" {
		t.Fatalf("payload = %#v, want example app info", payload)
	}
	if !jsonContainsTopLevelField(t, stdout.Bytes(), "source") {
		t.Fatalf("stdout = %q, want top-level source field", stdout.String())
	}
	if !jsonContainsTopLevelField(t, stdout.Bytes(), "update_source") {
		t.Fatalf("stdout = %q, want top-level update_source field", stdout.String())
	}
	if got, want := service.infoReq.Target, "example-app"; got != want {
		t.Fatalf("InfoRequest.Target = %q, want %q", got, want)
	}
}

func jsonContainsTopLevelField(t *testing.T, data []byte, field string) bool {
	t.Helper()

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, string(data))
	}
	_, ok := payload[field]
	return ok
}

func TestCommandReturnsServiceError(t *testing.T) {
	wantErr := errors.New("info failed")
	service := &fakeService{infoErr: wantErr}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"example-app"})

	err := cmd.ExecuteContext(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("ExecuteContext() error = %v, want %v", err, wantErr)
	}
}

func TestCommandHelpDescribesIntegratedAndLocalAppImages(t *testing.T) {
	cmd := NewCommand(clienv.New(&bytes.Buffer{}, &bytes.Buffer{}), &fakeService{})

	if got, want := cmd.Use, "info <app-id|path>"; got != want {
		t.Fatalf("Use = %q, want %q", got, want)
	}
	for _, want := range []string{"integrated AppImage", "local AppImage file", "executing the AppImage", "inspect only AppImages you trust"} {
		if !strings.Contains(cmd.Long, want) {
			t.Fatalf("Long = %q, want it to contain %q", cmd.Long, want)
		}
	}
}

type fakeService struct {
	infoCalled bool
	infoReq    app.InfoRequest
	infoResult app.InfoResult
	infoErr    error
}

var _ service = (*fakeService)(nil)

func (s *fakeService) Info(ctx context.Context, req app.InfoRequest) (app.InfoResult, error) {
	s.infoCalled = true
	s.infoReq = req
	if s.infoErr != nil {
		return app.InfoResult{}, s.infoErr
	}
	return s.infoResult, nil
}
