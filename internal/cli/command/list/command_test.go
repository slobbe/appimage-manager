package list

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aim/internal/app"
	"aim/internal/cli/clienv"
)

func TestCommandCallsServiceAndPrintsTextList(t *testing.T) {
	service := &fakeService{
		listResult: app.ListResult{Items: []app.ListItem{
			{ID: "example-app", Name: "Example App", Version: "1.2.3"},
			{ID: "other", Name: "Other", Version: "unknown"},
		}},
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

	if !service.listCalled {
		t.Fatal("service.List was not called")
	}
	output := stdout.String()
	for _, want := range []string{"ID", "Name", "Version", "example-app", "Example App", "1.2.3", "other", "Other", "unknown"} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want it to contain %q", output, want)
		}
	}
}

func TestCommandPrintsJSONList(t *testing.T) {
	service := &fakeService{
		listResult: app.ListResult{Items: []app.ListItem{
			{ID: "example-app", Name: "Example App", Version: "1.2.3"},
		}},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.JSON = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(nil)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	var payload struct {
		Items []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"items"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	if len(payload.Items) != 1 {
		t.Fatalf("len(payload.Items) = %d, want 1", len(payload.Items))
	}
	item := payload.Items[0]
	if item.ID != "example-app" || item.Name != "Example App" || item.Version != "1.2.3" {
		t.Fatalf("payload.Items[0] = %#v, want example app", item)
	}
}

func TestCommandReturnsServiceError(t *testing.T) {
	wantErr := errors.New("list failed")
	service := &fakeService{listErr: wantErr}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(nil)

	err := cmd.ExecuteContext(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("ExecuteContext() error = %v, want %v", err, wantErr)
	}
}

type fakeService struct {
	listCalled bool
	listResult app.ListResult
	listErr    error
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
	s.listCalled = true
	if s.listErr != nil {
		return app.ListResult{}, s.listErr
	}
	return s.listResult, nil
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
