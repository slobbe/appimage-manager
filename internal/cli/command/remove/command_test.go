package remove

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
			if service.removeCalled {
				t.Fatal("service.Remove was called for invalid args")
			}
		})
	}
}

func TestCommandPassesNameAndPrintsTextSuccess(t *testing.T) {
	service := &fakeService{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"example-app"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if got, want := service.removeReq.Name, "example-app"; got != want {
		t.Fatalf("RemoveRequest.Name = %q, want %q", got, want)
	}
	if service.removeReq.Activity == nil {
		t.Fatal("RemoveRequest.Activity = nil, want reporter")
	}
	if !strings.Contains(stdout.String(), "Successfully removed example-app!") {
		t.Fatalf("stdout = %q, want success message", stdout.String())
	}
}

func TestCommandPrintsJSONAndSuppressesActivityNoise(t *testing.T) {
	service := &fakeService{}
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
		Status string `json:"status"`
		Action string `json:"action"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	if payload.Status != "ok" || payload.Action != "remove" || payload.Name != "example-app" {
		t.Fatalf("payload = %#v, want ok remove example-app", payload)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no activity output in JSON mode", stderr.String())
	}
}

func TestCommandReturnsServiceError(t *testing.T) {
	wantErr := errors.New("remove failed")
	service := &fakeService{removeErr: wantErr}
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

type fakeService struct {
	removeCalled bool
	removeReq    app.RemoveRequest
	removeErr    error
}

var _ service = (*fakeService)(nil)

func (s *fakeService) Remove(ctx context.Context, req app.RemoveRequest) error {
	s.removeCalled = true
	s.removeReq = req
	if s.removeErr != nil {
		return s.removeErr
	}
	return nil
}
