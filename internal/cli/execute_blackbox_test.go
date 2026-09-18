package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
)

func TestExecuteSeparatesCommandOutputAndErrors(t *testing.T) {
	t.Run("help is stdout only", func(t *testing.T) {
		code, stdout, stderr := execute(t, context.Background(), []string{"--help"}, &listService{})

		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
		}
		if !strings.Contains(stdout, "Usage:") || !strings.Contains(stdout, "Available Commands:") {
			t.Fatalf("stdout = %q, want root help", stdout)
		}
		if stderr != "" {
			t.Fatalf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("ordinary error is stderr only", func(t *testing.T) {
		code, stdout, stderr := execute(t, context.Background(), []string{"not-a-command"}, &listService{})

		if code != clienv.ExitUsage {
			t.Fatalf("exit code = %d, want %d", code, clienv.ExitUsage)
		}
		if stdout != "" {
			t.Fatalf("stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, `unknown command "not-a-command"`) {
			t.Fatalf("stderr = %q, want unknown-command error", stderr)
		}
	})
}

func TestExecuteJSONKeepsStdoutMachineReadable(t *testing.T) {
	service := &listService{
		result: app.ListResult{Items: []app.ListItem{{
			ID: "example", Name: "Example", Version: "1.2.3",
		}}},
	}

	code, stdout, stderr := execute(t, context.Background(), []string{"--json", "list"}, service)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	var got app.ListResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v; stdout = %q", err, stdout)
	}
	if len(got.Items) != 1 || got.Items[0].ID != "example" {
		t.Fatalf("decoded stdout = %#v, want example item", got)
	}

	service.err = errors.New("repository unavailable")
	code, stdout, stderr = execute(t, context.Background(), []string{"--json", "list"}, service)
	if code != 1 {
		t.Fatalf("error exit code = %d, want 1", code)
	}
	if stdout != "" {
		t.Fatalf("error stdout = %q, want empty", stdout)
	}
	if stderr != "repository unavailable\n" {
		t.Fatalf("error stderr = %q, want service error", stderr)
	}
}

func TestExecuteHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := &listService{
		list: func(ctx context.Context, _ app.ListRequest) (app.ListResult, error) {
			return app.ListResult{}, ctx.Err()
		},
	}

	code, stdout, stderr := execute(t, ctx, []string{"list"}, service)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if stderr != context.Canceled.Error()+"\n" {
		t.Fatalf("stderr = %q, want cancellation error", stderr)
	}
}

func TestExecuteReturnsFailureForPartialUpdateAfterWritingJSON(t *testing.T) {
	service := &updateService{result: app.UpdateResult{
		Applied:      true,
		Checked:      2,
		AppliedCount: 1,
		Updates: []app.UpdateCandidate{{
			ID: "updated", CurrentVersion: "1.0.0", NewVersion: "2.0.0",
		}},
		Failures: []app.UpdateFailure{{AppID: "failed", Error: "download failed"}},
	}}

	code, stdout, stderr := execute(t, context.Background(), []string{"--json", "--yes", "update"}, service)

	if code != clienv.ExitFailure {
		t.Fatalf("exit code = %d, want %d", code, clienv.ExitFailure)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty because failure details are in JSON", stderr)
	}
	var payload struct {
		Status string `json:"status"`
		Failed int    `json:"failed"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("stdout is not JSON: %v; stdout = %q", err, stdout)
	}
	if payload.Status != "partial_failure" || payload.Failed != 1 {
		t.Fatalf("payload = %#v, want partial failure", payload)
	}
}

func TestExecuteHumanUpdateOutcomesAreAccurate(t *testing.T) {
	t.Run("skipped target is not reported up to date", func(t *testing.T) {
		service := &updateService{result: app.UpdateResult{
			Checked: 1,
			Skipped: []app.UpdateSkip{{
				AppID: "example", Reason: app.UpdateSkipReasonUnsupportedSource, SourceKind: "zsync",
			}},
		}}
		code, stdout, stderr := execute(t, context.Background(), []string{"update", "example"}, service)
		if code != clienv.ExitSuccess {
			t.Fatalf("exit code = %d, want success", code)
		}
		if !strings.Contains(stdout, "No supported update source") || strings.Contains(stdout, "up-to-date") {
			t.Fatalf("stdout = %q, want explicit skipped result", stdout)
		}
		if !strings.Contains(stderr, "unsupported_update_source") {
			t.Fatalf("stderr = %q, want skip reason", stderr)
		}
	})

	t.Run("target failure is not reported canceled or successful", func(t *testing.T) {
		service := &updateService{result: app.UpdateResult{
			Checked:  1,
			Updates:  []app.UpdateCandidate{{ID: "example", CurrentVersion: "1", NewVersion: "2"}},
			Failures: []app.UpdateFailure{{AppID: "example", Error: "install failed"}},
		}}
		code, stdout, stderr := execute(t, context.Background(), []string{"--yes", "update", "example"}, service)
		if code != clienv.ExitFailure {
			t.Fatalf("exit code = %d, want failure", code)
		}
		if !strings.Contains(stdout, "No updates were applied") ||
			strings.Contains(stdout, "canceled") ||
			strings.Contains(stdout, "Successfully") {
			t.Fatalf("stdout = %q, want accurate failure result", stdout)
		}
		if !strings.Contains(stderr, "install failed") {
			t.Fatalf("stderr = %q, want failure detail", stderr)
		}
	})
}

func TestExecuteDoesNotLeakGlobalFlagsBetweenRuns(t *testing.T) {
	service := &listService{}

	code, jsonOut, jsonErr := execute(t, context.Background(), []string{"--json", "list"}, service)
	if code != 0 || jsonErr != "" || !json.Valid([]byte(jsonOut)) {
		t.Fatalf("JSON run = (%d, %q, %q), want successful JSON", code, jsonOut, jsonErr)
	}

	code, textOut, textErr := execute(t, context.Background(), []string{"list"}, service)
	if code != 0 || textErr != "" {
		t.Fatalf("text run = (%d, %q, %q), want success", code, textOut, textErr)
	}
	if json.Valid([]byte(textOut)) || !strings.Contains(textOut, "ID") {
		t.Fatalf("second stdout = %q, want text table unaffected by first run", textOut)
	}
}

func execute(t *testing.T, ctx context.Context, args []string, service app.Service) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Execute(ctx, args, &stdout, &stderr, service, "test-version")
	return code, stdout.String(), stderr.String()
}

type listService struct {
	app.Service
	result app.ListResult
	err    error
	list   func(context.Context, app.ListRequest) (app.ListResult, error)
}

type updateService struct {
	app.Service
	result app.UpdateResult
}

func (s *updateService) Update(context.Context, app.UpdateRequest) (app.UpdateResult, error) {
	return s.result, nil
}

func (s *listService) List(ctx context.Context, req app.ListRequest) (app.ListResult, error) {
	if s.list != nil {
		return s.list(ctx, req)
	}
	return s.result, s.err
}
