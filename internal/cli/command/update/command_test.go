package update

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aim/internal/app"
	"aim/internal/cli/clienv"
)

func TestCommandPrintsAllAppsUpToDate(t *testing.T) {
	service := &fakeService{
		updateResult: app.UpdateResult{Applied: true},
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

	if got, want := stdout.String(), "All apps up-to-date\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestCommandPromptsAndPrintsUpdateCanceledWhenRejected(t *testing.T) {
	candidate := app.UpdateCandidate{ID: "example-app", CurrentVersion: "1.2.3", NewVersion: "2.0.0"}
	service := &fakeService{updateCandidates: []app.UpdateCandidate{candidate}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs(nil)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	output := stdout.String()
	for _, want := range []string{
		"[example-app]",
		"1.2.3 -> 2.0.0",
		"Update all apps? (y/n)",
		"Update canceled\n",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want it to contain %q", output, want)
		}
	}
	if !service.confirmationCalled {
		t.Fatal("service did not call update confirmation")
	}
}

func TestCommandPassesTargetAndPrintsTargetSuccess(t *testing.T) {
	candidate := app.UpdateCandidate{ID: "example-app", CurrentVersion: "1.2.3", NewVersion: "2.0.0"}
	service := &fakeService{updateCandidates: []app.UpdateCandidate{candidate}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetArgs([]string{"example-app"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if got, want := service.target, "example-app"; got != want {
		t.Fatalf("UpdateRequest.Target = %q, want %q", got, want)
	}
	if !strings.Contains(stdout.String(), "Successfully updated example-app!") {
		t.Fatalf("stdout = %q, want target success message", stdout.String())
	}
}

func TestCommandJSONIncludesTarget(t *testing.T) {
	service := &fakeService{updateResult: app.UpdateResult{Applied: true}}
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
		Status  string `json:"status"`
		Action  string `json:"action"`
		Target  string `json:"target"`
		Applied bool   `json:"applied"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	if payload.Status != "ok" || payload.Action != "update" || payload.Target != "example-app" || !payload.Applied {
		t.Fatalf("payload = %#v, want ok update target applied", payload)
	}
	if got, want := service.target, "example-app"; got != want {
		t.Fatalf("UpdateRequest.Target = %q, want %q", got, want)
	}
}

func TestCommandSetGitHubUpdateSource(t *testing.T) {
	service := &fakeService{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"--set", "example-app", "--github", "owner/repo", "--asset", "Example-*.AppImage", "--prerelease"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if got, want := service.setReq.ID, "example-app"; got != want {
		t.Fatalf("SetUpdateSourceRequest.ID = %q, want %q", got, want)
	}
	if got, want := service.setReq.GitHubRepo, "owner/repo"; got != want {
		t.Fatalf("SetUpdateSourceRequest.GitHubRepo = %q, want %q", got, want)
	}
	if got, want := service.setReq.AssetPattern, "Example-*.AppImage"; got != want {
		t.Fatalf("SetUpdateSourceRequest.AssetPattern = %q, want %q", got, want)
	}
	if !service.setReq.Prerelease {
		t.Fatal("SetUpdateSourceRequest.Prerelease = false, want true")
	}
	if service.setReq.Embedded {
		t.Fatal("SetUpdateSourceRequest.Embedded = true, want false")
	}
	if !strings.Contains(stdout.String(), "Set update source for example-app.") {
		t.Fatalf("stdout = %q, want set success message", stdout.String())
	}
}

func TestCommandSetEmbeddedUpdateSource(t *testing.T) {
	service := &fakeService{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"--set", "example-app", "--embedded"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if got, want := service.setReq.ID, "example-app"; got != want {
		t.Fatalf("SetUpdateSourceRequest.ID = %q, want %q", got, want)
	}
	if !service.setReq.Embedded {
		t.Fatal("SetUpdateSourceRequest.Embedded = false, want true")
	}
	if service.setReq.GitHubRepo != "" {
		t.Fatalf("SetUpdateSourceRequest.GitHubRepo = %q, want empty", service.setReq.GitHubRepo)
	}
}

func TestCommandUnsetUpdateSource(t *testing.T) {
	service := &fakeService{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"--unset", "example-app"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if got, want := service.unsetReq.ID, "example-app"; got != want {
		t.Fatalf("UnsetUpdateSourceRequest.ID = %q, want %q", got, want)
	}
	if !strings.Contains(stdout.String(), "Unset update source for example-app.") {
		t.Fatalf("stdout = %q, want unset success message", stdout.String())
	}
}

func TestCommandRejectsInvalidUpdateSourceFlagCombination(t *testing.T) {
	service := &fakeService{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"--set", "example-app", "--github", "owner/repo", "--embedded"})

	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Fatal("ExecuteContext() error = nil, want invalid flag combination error")
	}
}

func TestCommandJSONSetUpdateSource(t *testing.T) {
	service := &fakeService{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.JSON = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"--set", "example-app", "--github", "owner/repo"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	var payload struct {
		Status string `json:"status"`
		Action string `json:"action"`
		ID     string `json:"id"`
		Kind   string `json:"kind"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	if payload.Status != "ok" || payload.Action != "set_update_source" || payload.ID != "example-app" || payload.Kind != "github" {
		t.Fatalf("payload = %#v, want set update source payload", payload)
	}
}

func TestCommandJSONAutoConfirmsUpdates(t *testing.T) {
	candidate := app.UpdateCandidate{ID: "example-app", CurrentVersion: "1.2.3", NewVersion: "2.0.0"}
	service := &fakeService{updateCandidates: []app.UpdateCandidate{candidate}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.JSON = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs(nil)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if !service.confirmationCalled {
		t.Fatal("service did not call update confirmation")
	}
	if !service.confirmed {
		t.Fatal("confirmation = false, want true for JSON mode")
	}

	var payload struct {
		Status  string `json:"status"`
		Action  string `json:"action"`
		Applied bool   `json:"applied"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	if payload.Status != "ok" || payload.Action != "update" || !payload.Applied {
		t.Fatalf("payload = %#v, want ok update applied", payload)
	}
}

type fakeService struct {
	updateResult       app.UpdateResult
	updateCandidates   []app.UpdateCandidate
	updateErr          error
	target             string
	setReq             app.SetUpdateSourceRequest
	unsetReq           app.UnsetUpdateSourceRequest
	confirmationCalled bool
	confirmed          bool
}

var _ app.Service = (*fakeService)(nil)

func (s *fakeService) Add(ctx context.Context, req app.AddRequest) (app.AddResult, error) {
	return app.AddResult{}, nil
}

func (s *fakeService) Remove(ctx context.Context, req app.RemoveRequest) error {
	return nil
}

func (s *fakeService) SetUpdateSource(ctx context.Context, req app.SetUpdateSourceRequest) (app.SetUpdateSourceResult, error) {
	s.setReq = req
	result := app.SetUpdateSourceResult{ID: req.ID}
	if req.GitHubRepo != "" {
		result.UpdateSource.Kind = "github"
	} else if req.Embedded {
		result.UpdateSource.Kind = "github"
		result.UpdateSource.Embedded = true
	}
	return result, nil
}

func (s *fakeService) UnsetUpdateSource(ctx context.Context, req app.UnsetUpdateSourceRequest) error {
	s.unsetReq = req
	return nil
}

func (s *fakeService) Update(ctx context.Context, req app.UpdateRequest) (app.UpdateResult, error) {
	s.target = req.Target
	if s.updateErr != nil {
		return app.UpdateResult{}, s.updateErr
	}
	if len(s.updateCandidates) == 0 {
		return s.updateResult, nil
	}

	confirmed := true
	if req.Confirmation != nil {
		s.confirmationCalled = true
		var err error
		confirmed, err = req.Confirmation.ConfirmUpdates(ctx, s.updateCandidates)
		if err != nil {
			return app.UpdateResult{}, err
		}
	}
	s.confirmed = confirmed
	return app.UpdateResult{Applied: confirmed, Updates: s.updateCandidates}, nil
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
