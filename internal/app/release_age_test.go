package app

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slobbe/appimage-manager/internal/domain"
)

func TestShouldDeferRelease(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 123, time.UTC)
	stable := domain.NewGitHubUpdateSource("owner/app", false)
	manualPre := domain.NewGitHubUpdateSource("owner/app", true)
	embedded := func(tag string) domain.UpdateSource {
		source := manualPre
		source.Embedded, source.ReleaseTag = true, tag
		return source
	}
	for _, tc := range []struct {
		name                                  string
		source                                domain.UpdateSource
		published                             time.Time
		hours                                 int64
		ignore, prerelease, deferred, failure bool
	}{
		{name: "just younger", source: stable, published: now.Add(-24*time.Hour + time.Nanosecond), hours: 24, deferred: true},
		{name: "boundary", source: stable, published: now.Add(-24 * time.Hour), hours: 24},
		{name: "older", source: stable, published: now.Add(-24*time.Hour - time.Nanosecond), hours: 24},
		{name: "future", source: stable, published: now.Add(time.Hour), hours: 24, deferred: true},
		{name: "missing", source: stable, hours: 24, failure: true},
		{name: "disabled missing", source: stable},
		{name: "override missing", source: stable, hours: 24, ignore: true},
		{name: "manual prerelease stable selection", source: manualPre, hours: 24},
		{name: "embedded latest-pre", source: embedded("latest-pre"), hours: 24},
		{name: "embedded latest-all", source: embedded("latest-all"), hours: 24},
		{name: "embedded latest ignores incidental flag", source: embedded("latest"), published: now, hours: 24, deferred: true},
		{name: "embedded stable tag ignores incidental flag", source: embedded("v2.0.0"), published: now, hours: 24, deferred: true},
		{name: "exact tag actual prerelease", source: embedded("v2.0.0-rc1"), hours: 24, prerelease: true},
		{name: "actual prerelease", source: stable, hours: 24, prerelease: true},
		{name: "maximum hours", source: stable, published: now.Add(-48 * time.Hour), hours: math.MaxInt64, deferred: true},
		{name: "duration overflow hours", source: stable, published: now.Add(-48 * time.Hour), hours: 1 << 32, deferred: true},
		{name: "age beyond duration range", source: stable, published: now.AddDate(-500, 0, 0), hours: 400 * 366 * 24},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := shouldDeferRelease(tc.source, GitHubRelease{PublishedAt: tc.published, Prerelease: tc.prerelease}, tc.hours, tc.ignore, now)
			if got != tc.deferred || (err != nil) != tc.failure {
				t.Fatalf("deferred=%v err=%v; want deferred=%v failure=%v", got, err, tc.deferred, tc.failure)
			}
		})
	}
}

func TestReleaseAgeBulkPlanning(t *testing.T) {
	apps := concurrentChecksApps(6)
	apps[4].Version, _ = domain.ParseVersion("2.0.0")
	apps[5].UpdateSource.Prerelease = true
	published := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	var calls atomic.Int32
	s := &service{
		config: Config{MinimumReleaseAgeHours: 24}, apps: concurrentChecksRepository{apps: apps},
		githubReleases: concurrentChecksFinder(func(_ context.Context, repo string) (GitHubRelease, error) {
			calls.Add(1)
			release := concurrentChecksRelease()
			switch repo {
			case apps[0].ID, apps[3].ID:
				release.PublishedAt = published
			case apps[1].ID:
				release.PublishedAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			return release, nil
		}),
	}
	plans, candidates, failures, skips, deferred, checked, err := s.planGitHubUpdates(context.Background(), "", NoopActivityReporter{}, false)
	if err != nil || checked != 6 || len(skips) != 0 || len(plans) != 2 || len(candidates) != 2 || candidates[0].ID != apps[1].ID || candidates[1].ID != apps[5].ID {
		t.Fatalf("plans=%v candidates=%v failures=%v skips=%v checked=%d err=%v", plans, candidates, failures, skips, checked, err)
	}
	if len(failures) != 1 || failures[0].AppID != apps[2].ID || !strings.Contains(failures[0].Error, "publication time") {
		t.Fatalf("failures=%v", failures)
	}
	want := []UpdateDeferred{
		{AppID: apps[0].ID, CurrentVersion: "1.0.0", NewVersion: "2.0.0", PublishedAt: published, MinimumReleaseAgeHours: 24},
		{AppID: apps[3].ID, CurrentVersion: "1.0.0", NewVersion: "2.0.0", PublishedAt: published, MinimumReleaseAgeHours: 24},
	}
	if !reflect.DeepEqual(deferred, want) || calls.Load() != 6 {
		t.Fatalf("deferred=%v want=%v calls=%d", deferred, want, calls.Load())
	}
}

func TestReleaseAgeUpdateReturnPaths(t *testing.T) {
	for _, mode := range []string{"check", "no plan", "declined", "confirmation error", "confirmation cancellation", "planning cancellation", "apply download failure", "override"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			apps := concurrentChecksApps(2)
			if mode == "no plan" || mode == "planning cancellation" {
				apps = apps[:1]
			}
			mutations := &concurrentChecksMutationSpy{}
			confirmations := 0
			s := &service{
				config: Config{MinimumReleaseAgeHours: 24}, apps: concurrentChecksRepository{apps: apps},
				mutations: &concurrentChecksLocker{}, downloads: mutations,
				githubReleases: concurrentChecksFinder(func(_ context.Context, repo string) (GitHubRelease, error) {
					release := concurrentChecksRelease()
					release.PublishedAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
					if repo == apps[0].ID {
						release.PublishedAt = time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
					}
					if mode == "planning cancellation" {
						cancel()
					}
					return release, nil
				}),
			}
			result, err := s.Update(ctx, UpdateRequest{
				CheckOnly: mode == "check" || mode == "override", IgnoreReleaseAge: mode == "override",
				Confirmation: concurrentChecksConfirmation(func(_ context.Context, candidates []UpdateCandidate) (bool, error) {
					confirmations++
					if len(candidates) != 1 || candidates[0].ID != apps[1].ID {
						t.Errorf("confirmation candidates=%v", candidates)
					}
					switch mode {
					case "confirmation error":
						return false, errors.New("confirmation failed")
					case "confirmation cancellation":
						cancel()
						return false, ctx.Err()
					case "apply download failure":
						return true, nil
					default:
						return false, nil
					}
				}),
			})
			wantError := strings.Contains(mode, "cancellation") || mode == "confirmation error"
			if (err != nil) != wantError {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if mode == "override" {
				if len(result.Deferred) != 0 || len(result.Updates) != 2 {
					t.Fatalf("result=%+v", result)
				}
			} else if len(result.Deferred) != 1 || result.Deferred[0].AppID != apps[0].ID {
				t.Fatalf("lost deferred result: %+v err=%v", result, err)
			}
			wantDownloads := int32(0)
			if mode == "apply download failure" {
				wantDownloads = 1
				if len(result.Failures) != 1 || result.Failures[0].AppID != apps[1].ID {
					t.Fatalf("failures=%v", result.Failures)
				}
			}
			if mutations.downloads.Load() != wantDownloads {
				t.Fatalf("downloads=%d want=%d", mutations.downloads.Load(), wantDownloads)
			}
			if (mode == "check" || mode == "no plan" || mode == "planning cancellation" || mode == "override") && confirmations != 0 {
				t.Fatal("unexpected confirmation")
			}
		})
	}
}
