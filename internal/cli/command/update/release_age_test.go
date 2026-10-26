package update

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
)

func TestCommandForwardsIgnoreReleaseAge(t *testing.T) {
	for _, args := range [][]string{nil, {"example-app"}, {"--check"}, {"--check", "example-app"}} {
		for _, flag := range []string{"", "--ignore-release-age", "--ignore-release-age=false"} {
			t.Run(strings.Join(append(append([]string{}, args...), flag), " "), func(t *testing.T) {
				service := &fakeService{}
				stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
				cmd := NewCommand(clienv.New(stdout, stderr), service)
				cmd.SetOut(stdout)
				cmd.SetErr(stderr)
				commandArgs := append([]string{}, args...)
				if flag != "" {
					commandArgs = append(commandArgs, flag)
				}
				cmd.SetArgs(commandArgs)
				if err := cmd.ExecuteContext(context.Background()); err != nil {
					t.Fatal(err)
				}
				if !service.updateCalled || service.ignoreReleaseAge != (flag == "--ignore-release-age") {
					t.Fatalf("Update called = %v, IgnoreReleaseAge = %v", service.updateCalled, service.ignoreReleaseAge)
				}
			})
		}
	}
}

func TestCommandRejectsIgnoreReleaseAgeWithSourceFlags(t *testing.T) {
	for _, sourceArgs := range [][]string{
		{"--set", "example-app", "--github", "owner/repo"},
		{"--set", "example-app", "--embedded"},
		{"--unset", "example-app"},
		{"--set="}, {"--unset="}, {"--github="}, {"--asset="},
		{"--exclude="}, {"--prefer="}, {"--embedded=false"}, {"--prerelease=false"},
	} {
		for _, flag := range []string{"--ignore-release-age", "--ignore-release-age=false"} {
			args := append(append([]string{}, sourceArgs...), flag)
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				service := &fakeService{}
				stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
				cmd := NewCommand(clienv.New(stdout, stderr), service)
				cmd.SetOut(stdout)
				cmd.SetErr(stderr)
				cmd.SetArgs(args)
				if err := cmd.ExecuteContext(context.Background()); err == nil || !strings.Contains(err.Error(), "--ignore-release-age cannot be combined with update source flags") {
					t.Fatalf("ExecuteContext() error = %v", err)
				}
				if service.updateCalled || !reflect.DeepEqual(service.setReq, app.SetUpdateSourceRequest{}) || service.unsetReq.ID != "" {
					t.Fatal("service was called for invalid flags")
				}
			})
		}
	}
}

func TestCommandReleaseAgeResults(t *testing.T) {
	deferred := app.UpdateDeferred{
		AppID: "waiting", CurrentVersion: "1.0.0", NewVersion: "2.0.0",
		PublishedAt: time.Now().Add(-30 * time.Minute).UTC().Truncate(time.Second), MinimumReleaseAgeHours: 1,
	}
	candidate := app.UpdateCandidate{ID: "ready", CurrentVersion: "1.0.0", NewVersion: "2.0.0"}
	for _, tc := range []struct {
		name   string
		check  bool
		result app.UpdateResult
		status string
		want   string
	}{
		{"empty", false, app.UpdateResult{}, "up_to_date", "All apps up-to-date"},
		{"deferred", false, app.UpdateResult{Checked: 1, Deferred: []app.UpdateDeferred{deferred}}, "deferred", "Deferred [waiting]: 1.0.0 -> 2.0.0"},
		{"check deferred", true, app.UpdateResult{Checked: 1, Deferred: []app.UpdateDeferred{deferred}}, "deferred", "Deferred [waiting]: 1.0.0 -> 2.0.0"},
		{"skipped and deferred", false, app.UpdateResult{Checked: 2, Deferred: []app.UpdateDeferred{deferred}, Skipped: []app.UpdateSkip{{AppID: "unsupported", Reason: app.UpdateSkipReasonUnsupportedSource}}}, "deferred", "Deferred [waiting]"},
		{"available and deferred", true, app.UpdateResult{Checked: 2, Deferred: []app.UpdateDeferred{deferred}, Updates: []app.UpdateCandidate{candidate}}, "updates_available", "Updates available:"},
		{"updated and deferred", false, app.UpdateResult{Checked: 2, Applied: true, AppliedCount: 1, Deferred: []app.UpdateDeferred{deferred}, Updates: []app.UpdateCandidate{candidate}}, "updated", "Updated 1 app(s); 1 deferred."},
		{"warnings and deferred", false, app.UpdateResult{Checked: 2, Applied: true, AppliedCount: 1, Deferred: []app.UpdateDeferred{deferred}, Updates: []app.UpdateCandidate{candidate}, Warnings: []app.OperationWarning{{}}}, "updated_with_warnings", "Updated 1 app(s); 1 deferred."},
		{"canceled and deferred", false, app.UpdateResult{Checked: 2, Deferred: []app.UpdateDeferred{deferred}, Updates: []app.UpdateCandidate{candidate}}, "canceled", "Update canceled"},
		{"failure and deferred", false, app.UpdateResult{Checked: 2, Deferred: []app.UpdateDeferred{deferred}, Failures: []app.UpdateFailure{{AppID: "broken", Error: "check failed"}}}, "partial_failure", "No updates were applied; 1 app(s) failed."},
		{"check failure and deferred", true, app.UpdateResult{Checked: 2, Deferred: []app.UpdateDeferred{deferred}, Failures: []app.UpdateFailure{{AppID: "broken", Error: "check failed"}}}, "partial_failure", "No eligible updates found"},
	} {
		for _, jsonOutput := range []bool{false, true} {
			t.Run(tc.name+"/json="+strconv.FormatBool(jsonOutput), func(t *testing.T) {
				service := &fakeService{updateResult: tc.result}
				stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
				rt := clienv.New(stdout, stderr)
				rt.Config.JSON = jsonOutput
				cmd := NewCommand(rt, service)
				cmd.SetOut(stdout)
				cmd.SetErr(stderr)
				args := []string{}
				if tc.check {
					args = append(args, "--check")
				}
				cmd.SetArgs(args)
				err := cmd.ExecuteContext(context.Background())
				if len(tc.result.Failures) > 0 {
					assertSilentFailure(t, err)
				} else if err != nil {
					t.Fatal(err)
				}
				if jsonOutput {
					var payload struct {
						Status    string               `json:"status"`
						Available int                  `json:"available"`
						Deferred  []app.UpdateDeferred `json:"deferred"`
					}
					if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
						t.Fatal(err)
					}
					if payload.Status != tc.status || payload.Available != len(tc.result.Updates) || payload.Deferred == nil || !reflect.DeepEqual(payload.Deferred, itemsOrEmpty(tc.result.Deferred)) {
						t.Fatalf("payload = %+v, want status %s, available %d, deferred %+v", payload, tc.status, len(tc.result.Updates), tc.result.Deferred)
					}
					if len(payload.Deferred) > 0 {
						var raw struct {
							Deferred []map[string]json.RawMessage `json:"deferred"`
						}
						if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
							t.Fatal(err)
						}
						for _, key := range []string{"app_id", "current_version", "new_version", "published_at", "minimum_release_age_hours"} {
							if _, ok := raw.Deferred[0][key]; !ok {
								t.Fatalf("deferred JSON missing %q: %s", key, stdout.String())
							}
						}
					}
					if stderr.Len() != 0 {
						t.Fatalf("JSON stderr = %q", stderr.String())
					}
					return
				}
				text := stdout.String()
				if !strings.Contains(text, tc.want) {
					t.Fatalf("stdout = %q, want %q", text, tc.want)
				}
				if len(tc.result.Deferred) > 0 {
					for _, want := range []string{"Deferred [waiting]: 1.0.0 -> 2.0.0", "remaining before eligible", "--ignore-release-age"} {
						if !strings.Contains(text, want) {
							t.Fatalf("stdout = %q, want %q", text, want)
						}
					}
					for _, unwanted := range []string{"All apps up-to-date", "Successfully updated all apps", "No supported update source", "No updates found"} {
						if strings.Contains(text, unwanted) {
							t.Fatalf("stdout = %q contains misleading %q", text, unwanted)
						}
					}
				}
			})
		}
	}
}

func TestReleaseAgeRemaining(t *testing.T) {
	now := time.Date(2026, time.October, 26, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		hours     int64
		published time.Time
		want      string
	}{
		{"half hour", 1, now.Add(-30 * time.Minute), "0h 30m"},
		{"round up", 1, now.Add(-30*time.Minute + time.Nanosecond), "0h 31m"},
		{"elapsed", 1, now.Add(-time.Hour), "0m"},
		{"future", 1, now.Add(30 * time.Minute), "1h 30m"},
		{"max hours", math.MaxInt64, now.Add(-30 * time.Minute), "9223372036854775806h 30m"},
		{"max hours future", math.MaxInt64, now.Add(30 * time.Minute), "9223372036854775807h 30m"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := releaseAgeRemaining(app.UpdateDeferred{PublishedAt: tc.published, MinimumReleaseAgeHours: tc.hours}, now); got != tc.want {
				t.Fatalf("releaseAgeRemaining() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWriteUpdateDeferralsListsEachApp(t *testing.T) {
	now := time.Date(2026, time.October, 26, 12, 0, 0, 0, time.UTC)
	deferred := []app.UpdateDeferred{
		{AppID: "first", CurrentVersion: "1.0.0", NewVersion: "2.0.0", PublishedAt: now.Add(-30 * time.Minute), MinimumReleaseAgeHours: 1},
		{AppID: "second", CurrentVersion: "2.0.0", NewVersion: "3.0.0", PublishedAt: now.Add(-30 * time.Minute), MinimumReleaseAgeHours: math.MaxInt64},
	}
	var output bytes.Buffer
	if err := writeUpdateDeferrals(&output, deferred, now); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Deferred [first]: 1.0.0 -> 2.0.0 (0h 30m remaining before eligible).",
		"Deferred [second]: 2.0.0 -> 3.0.0 (9223372036854775806h 30m remaining before eligible).",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output = %q, want %q", output.String(), want)
		}
	}
	if strings.Count(output.String(), "--ignore-release-age") != 1 {
		t.Fatalf("output = %q, want one bypass hint", output.String())
	}
}

func TestWriteUpdateDeferralsReturnsWriterError(t *testing.T) {
	wantErr := context.Canceled
	if err := writeUpdateDeferrals(failingWriter{err: wantErr}, []app.UpdateDeferred{{}}, time.Now()); err != wantErr {
		t.Fatalf("writeUpdateDeferrals() error = %v, want %v", err, wantErr)
	}
}
