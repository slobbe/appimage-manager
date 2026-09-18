package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
)

func TestCommandPrintsAllAppsUpToDate(t *testing.T) {
	service := &fakeService{
		updateResult: app.UpdateResult{Applied: true},
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.NonInteractive = true
	cmd := NewCommand(rt, service)
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

func TestCommandCheckOnlyPrintsPendingUpdatesWithoutPrompting(t *testing.T) {
	candidate := app.UpdateCandidate{ID: "example-app", CurrentVersion: "1.2.3", NewVersion: "2.0.0"}
	service := &fakeService{updateCandidates: []app.UpdateCandidate{candidate}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--check"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if !service.checkOnly {
		t.Fatal("UpdateRequest.CheckOnly = false, want true")
	}
	if service.confirmationCalled {
		t.Fatal("service called update confirmation for check-only update")
	}
	output := stdout.String()
	for _, want := range []string{
		"Updates available:\n",
		"[example-app]",
		"1.2.3 -> 2.0.0",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout = %q, want it to contain %q", output, want)
		}
	}
	if strings.Contains(output, "Successfully updated") || strings.Contains(output, "Update canceled") || strings.Contains(output, "Update all apps?") {
		t.Fatalf("stdout = %q, want check-only output without apply/cancel/prompt wording", output)
	}
}

func TestCommandJSONCheckOnlyIncludesUpdatesWithoutPrompting(t *testing.T) {
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
	cmd.SetArgs([]string{"--check", "example-app"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	var payload struct {
		Status  string                `json:"status"`
		Action  string                `json:"action"`
		Target  string                `json:"target"`
		Applied bool                  `json:"applied"`
		Updates []app.UpdateCandidate `json:"updates"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	if payload.Status != "updates_available" || payload.Action != "update" || payload.Target != "example-app" || payload.Applied {
		t.Fatalf("payload = %#v, want available update target not applied", payload)
	}
	if len(payload.Updates) != 1 || payload.Updates[0] != candidate {
		t.Fatalf("payload updates = %#v, want %#v", payload.Updates, []app.UpdateCandidate{candidate})
	}
	if !service.checkOnly {
		t.Fatal("UpdateRequest.CheckOnly = false, want true")
	}
	if service.confirmationCalled {
		t.Fatal("service called update confirmation for JSON check-only update")
	}
}

func TestCommandJSONStillRequiresConfirmation(t *testing.T) {
	candidate := app.UpdateCandidate{ID: "example-app", CurrentVersion: "1.2.3", NewVersion: "2.0.0"}
	service := &fakeService{updateCandidates: []app.UpdateCandidate{candidate}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.JSON = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader("n\n"))

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	var payload struct {
		Applied bool `json:"applied"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	if payload.Applied {
		t.Fatal("payload.Applied = true, want false after rejecting JSON-mode confirmation")
	}
	if service.confirmed {
		t.Fatal("confirmation = true, want false; --json must not auto-confirm")
	}
	if strings.Contains(stdout.String(), "Update all apps?") {
		t.Fatalf("stdout = %q, want JSON only", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Update all apps? (y/n)") {
		t.Fatalf("stderr = %q, want confirmation prompt", stderr.String())
	}
}

func TestCommandYesAutoConfirmsWithoutReadingInput(t *testing.T) {
	candidate := app.UpdateCandidate{ID: "example-app", CurrentVersion: "1.2.3", NewVersion: "2.0.0"}
	service := &fakeService{updateCandidates: []app.UpdateCandidate{candidate}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.Yes = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	if !service.confirmed {
		t.Fatal("confirmation = false, want true with --yes")
	}
	if strings.Contains(stdout.String(), "(y/n)") {
		t.Fatalf("stdout = %q, want no prompt with --yes", stdout.String())
	}
}

func TestCommandNonInteractiveFailsWhenConfirmationIsRequired(t *testing.T) {
	candidate := app.UpdateCandidate{ID: "example-app", CurrentVersion: "1.2.3", NewVersion: "2.0.0"}
	service := &fakeService{updateCandidates: []app.UpdateCandidate{candidate}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.JSON = true
	rt.Config.NonInteractive = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SilenceUsage = true

	err := cmd.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "confirmation required in non-interactive mode; rerun with --yes") {
		t.Fatalf("ExecuteContext() error = %v, want non-interactive confirmation error", err)
	}
	if service.confirmed {
		t.Fatal("confirmation = true, want false")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no partial JSON", stdout.String())
	}
}

func TestCommandYesWorksWithNonInteractive(t *testing.T) {
	candidate := app.UpdateCandidate{ID: "example-app", CurrentVersion: "1.2.3", NewVersion: "2.0.0"}
	service := &fakeService{updateCandidates: []app.UpdateCandidate{candidate}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.Yes = true
	rt.Config.NonInteractive = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if !service.confirmed {
		t.Fatal("confirmation = false, want true with --yes --non-interactive")
	}
}

func TestCommandCheckOnlyWorksWithNonInteractiveWithoutYes(t *testing.T) {
	candidate := app.UpdateCandidate{ID: "example-app", CurrentVersion: "1.2.3", NewVersion: "2.0.0"}
	service := &fakeService{updateCandidates: []app.UpdateCandidate{candidate}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.NonInteractive = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetArgs([]string{"--check"})

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}
	if service.confirmationCalled {
		t.Fatal("service called confirmation for --check --non-interactive")
	}
}

func TestCommandJSONIncludesTarget(t *testing.T) {
	service := &fakeService{updateResult: app.UpdateResult{Checked: 1}}
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
	if payload.Status != "up_to_date" || payload.Action != "update" || payload.Target != "example-app" || payload.Applied {
		t.Fatalf("payload = %#v, want up-to-date target", payload)
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

func TestCommandReportsPartialBulkUpdateAndReturnsSilentFailure(t *testing.T) {
	service := &fakeService{updateResult: app.UpdateResult{
		Applied:      true,
		Checked:      2,
		AppliedCount: 1,
		Updates:      []app.UpdateCandidate{{ID: "helium", CurrentVersion: "1.0.0", NewVersion: "2.0.0"}},
		Failures:     []app.UpdateFailure{{AppID: "localsend", Error: "release has no AppImage assets"}},
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(nil)

	assertSilentFailure(t, cmd.ExecuteContext(context.Background()))

	if !strings.Contains(stdout.String(), "Updated 1 app(s); 1 failed.") {
		t.Fatalf("stdout = %q, want partial success message", stdout.String())
	}
	if got, want := stderr.String(), "Update error [localsend]: release has no AppImage assets\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestCommandReturnsWriterErrorForFailuresWithoutUpdates(t *testing.T) {
	wantErr := errors.New("write failed")
	service := &fakeService{updateResult: app.UpdateResult{
		Applied:  true,
		Failures: []app.UpdateFailure{{AppID: "localsend", Error: "release has no AppImage assets"}},
	}}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(failingWriter{err: wantErr}, stderr), service)
	cmd.SetOut(failingWriter{err: wantErr})
	cmd.SetErr(stderr)
	cmd.SetArgs(nil)

	err := cmd.ExecuteContext(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("ExecuteContext() error = %v, want %v", err, wantErr)
	}
}

func TestCommandReturnsWriterErrorForSuccessfulOutput(t *testing.T) {
	wantErr := errors.New("write failed")
	service := &fakeService{updateResult: app.UpdateResult{Checked: 1}}
	cmd := NewCommand(clienv.New(failingWriter{err: wantErr}, io.Discard), service)
	cmd.SetOut(failingWriter{err: wantErr})

	err := cmd.ExecuteContext(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("ExecuteContext() error = %v, want %v", err, wantErr)
	}
}

func TestCommandCheckReportsCandidatesAndFailures(t *testing.T) {
	service := &fakeService{updateResult: app.UpdateResult{
		Checked: 2,
		Updates: []app.UpdateCandidate{{
			ID: "ready", CurrentVersion: "1.0.0", NewVersion: "2.0.0",
		}},
		Failures: []app.UpdateFailure{{AppID: "broken", Error: "check failed"}},
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd := NewCommand(clienv.New(stdout, stderr), service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs([]string{"--check"})

	assertSilentFailure(t, cmd.ExecuteContext(context.Background()))
	if !strings.Contains(stdout.String(), "Updates available:") ||
		!strings.Contains(stdout.String(), "[ready]") ||
		!strings.Contains(stdout.String(), "1 app update check(s) failed.") {
		t.Fatalf("stdout = %q, want candidates and partial check summary", stdout.String())
	}
}

func TestCommandJSONIncludesBulkUpdateFailures(t *testing.T) {
	failure := app.UpdateFailure{AppID: "localsend", Error: "release has no AppImage assets"}
	service := &fakeService{updateResult: app.UpdateResult{Applied: true, Checked: 1, Failures: []app.UpdateFailure{failure}}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.JSON = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetArgs(nil)

	assertSilentFailure(t, cmd.ExecuteContext(context.Background()))

	var payload struct {
		Status   string              `json:"status"`
		Failures []app.UpdateFailure `json:"failures"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	if payload.Status != "failed" || len(payload.Failures) != 1 || payload.Failures[0] != failure {
		t.Fatalf("payload = %#v, want failed with failure %#v", payload, failure)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty JSON stderr", stderr.String())
	}
}

func TestCommandJSONReportsSkippedUpdateSources(t *testing.T) {
	skip := app.UpdateSkip{
		AppID:      "example-app",
		Reason:     app.UpdateSkipReasonUnsupportedSource,
		SourceKind: "zsync",
	}
	service := &fakeService{updateResult: app.UpdateResult{
		Applied: true,
		Checked: 1,
		Skipped: []app.UpdateSkip{skip},
	}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.JSON = true
	cmd := NewCommand(rt, service)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("ExecuteContext() error = %v", err)
	}

	var payload struct {
		Status  string           `json:"status"`
		Skipped []app.UpdateSkip `json:"skipped"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	if payload.Status != "skipped" || len(payload.Skipped) != 1 || payload.Skipped[0] != skip {
		t.Fatalf("payload = %#v, want skipped source %#v", payload, skip)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestCommandJSONWithYesAutoConfirmsUpdates(t *testing.T) {
	candidate := app.UpdateCandidate{ID: "example-app", CurrentVersion: "1.2.3", NewVersion: "2.0.0"}
	service := &fakeService{updateCandidates: []app.UpdateCandidate{candidate}}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	rt := clienv.New(stdout, stderr)
	rt.Config.JSON = true
	rt.Config.Yes = true
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
		t.Fatal("confirmation = false, want true for JSON mode with --yes")
	}

	var payload struct {
		Status  string `json:"status"`
		Action  string `json:"action"`
		Applied bool   `json:"applied"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; stdout = %q", err, stdout.String())
	}
	if payload.Status != "updated" || payload.Action != "update" || !payload.Applied {
		t.Fatalf("payload = %#v, want updated result", payload)
	}
}

func assertSilentFailure(t *testing.T, err error) {
	t.Helper()
	code, message := clienv.ResolveExit(err)
	if code != clienv.ExitFailure || message != nil {
		t.Fatalf("ResolveExit(%v) = (%d, %v), want silent failure", err, code, message)
	}
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write(p []byte) (int, error) {
	return 0, w.err
}

type fakeService struct {
	updateResult       app.UpdateResult
	updateCandidates   []app.UpdateCandidate
	updateErr          error
	target             string
	checkOnly          bool
	setReq             app.SetUpdateSourceRequest
	unsetReq           app.UnsetUpdateSourceRequest
	confirmationCalled bool
	confirmed          bool
}

var _ service = (*fakeService)(nil)

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
	s.checkOnly = req.CheckOnly
	if s.updateErr != nil {
		return app.UpdateResult{}, s.updateErr
	}
	if len(s.updateCandidates) == 0 {
		return s.updateResult, nil
	}
	if req.CheckOnly {
		return app.UpdateResult{Applied: false, Checked: len(s.updateCandidates), Updates: s.updateCandidates}, nil
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
	result := app.UpdateResult{Applied: confirmed, Checked: len(s.updateCandidates), Updates: s.updateCandidates}
	if confirmed {
		result.AppliedCount = len(s.updateCandidates)
	}
	return result, nil
}
