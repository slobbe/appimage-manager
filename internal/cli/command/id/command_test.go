package id

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

func TestCommandValidatesFlags(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "missing flag", args: []string{"example-app"}},
		{name: "conflicting flags", args: []string{"example-app", "--set", "custom-id", "--auto"}},
		{name: "missing app id", args: []string{"--set", "custom-id"}},
		{name: "too many args", args: []string{"example-app", "extra", "--auto"}},
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
				t.Fatal("ExecuteContext() error = nil, want validation error")
			}
			if service.called {
				t.Fatal("service.SetID called for invalid args")
			}
		})
	}
}

func TestCommandPassesSetRequestAndPrintsTextSuccess(t *testing.T) {
	service := &fakeService{result: app.SetIDResult{PreviousID: "old-id", ID: "new-id", Changed: true}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"old-id", "--set", "new-id"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if got, want := service.req.CurrentID, "old-id"; got != want {
		t.Fatalf("CurrentID = %q, want %q", got, want)
	}
	if got, want := service.req.NewID, "new-id"; got != want {
		t.Fatalf("NewID = %q, want %q", got, want)
	}
	if service.req.Auto {
		t.Fatal("Auto = true, want false")
	}
	if service.req.Activity == nil {
		t.Fatal("Activity = nil, want reporter")
	}
	if !strings.Contains(stdout.String(), "Changed app ID from old-id to new-id") {
		t.Fatalf("stdout = %q, want changed message", stdout.String())
	}
}

func TestCommandPassesAutoRequest(t *testing.T) {
	service := &fakeService{result: app.SetIDResult{PreviousID: "old-id", ID: "auto-id", Changed: true}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"old-id", "--auto"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !service.req.Auto {
		t.Fatal("Auto = false, want true")
	}
	if service.req.NewID != "" {
		t.Fatalf("NewID = %q, want empty", service.req.NewID)
	}
}

func TestCommandPrintsJSON(t *testing.T) {
	service := &fakeService{result: app.SetIDResult{PreviousID: "old-id", ID: "new-id", Changed: true}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.JSON = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"old-id", "--set", "new-id"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	var payload struct {
		Status     string `json:"status"`
		Action     string `json:"action"`
		PreviousID string `json:"previous_id"`
		ID         string `json:"id"`
		Changed    bool   `json:"changed"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	if payload.Status != "ok" || payload.Action != "set_id" || payload.PreviousID != "old-id" || payload.ID != "new-id" || !payload.Changed {
		t.Fatalf("payload = %#v, want set_id result", payload)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no activity output in JSON mode", stderr.String())
	}
}

func TestCommandReturnsServiceError(t *testing.T) {
	wantErr := errors.New("set id failed")
	service := &fakeService{err: wantErr}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"old-id", "--set", "new-id"})

	err := cmd.ExecuteContext(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("ExecuteContext() error = %v, want %v", err, wantErr)
	}
}

type fakeService struct {
	called bool
	req    app.SetIDRequest
	result app.SetIDResult
	err    error
}

var _ app.IDManager = (*fakeService)(nil)

func (s *fakeService) SetID(ctx context.Context, req app.SetIDRequest) (app.SetIDResult, error) {
	s.called = true
	s.req = req
	if s.err != nil {
		return app.SetIDResult{}, s.err
	}
	return s.result, nil
}
