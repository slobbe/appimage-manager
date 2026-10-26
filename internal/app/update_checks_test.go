package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/slobbe/appimage-manager/internal/domain"
)

// Embedding the port makes any unexpected mutation fail immediately.
type concurrentChecksRepository struct {
	AppRepository
	apps []domain.App
	err  error
}

func (r concurrentChecksRepository) List(context.Context) ([]domain.App, error) {
	return r.apps, r.err
}

func (r concurrentChecksRepository) Find(_ context.Context, id string) (domain.App, error) {
	for _, installed := range r.apps {
		if installed.ID == id {
			return installed, r.err
		}
	}
	return domain.App{}, ErrAppNotFound
}

type concurrentChecksFinder func(context.Context, string) (GitHubRelease, error)

func (f concurrentChecksFinder) LatestRelease(ctx context.Context, repo string, _ bool) (GitHubRelease, error) {
	return f(ctx, repo)
}
func (f concurrentChecksFinder) LatestPrerelease(ctx context.Context, repo string) (GitHubRelease, error) {
	return f(ctx, repo)
}
func (f concurrentChecksFinder) ReleaseByTag(ctx context.Context, repo, _ string) (GitHubRelease, error) {
	return f(ctx, repo)
}

type concurrentChecksActivity struct {
	NoopActivityTask
	activities []Activity
	done       []string
	failed     []error
}

func (a *concurrentChecksActivity) Start(_ context.Context, activity Activity) ActivityTask {
	a.activities = append(a.activities, activity)
	return a
}
func (a *concurrentChecksActivity) Done(message string) { a.done = append(a.done, message) }
func (a *concurrentChecksActivity) Fail(err error)      { a.failed = append(a.failed, err) }

func concurrentChecksApps(count int) []domain.App {
	version, _ := domain.ParseVersion("1.0.0")
	apps := make([]domain.App, count)
	for i := range apps {
		id := fmt.Sprintf("app-%d", i)
		apps[i] = domain.App{ID: id, Version: version, UpdateSource: domain.NewGitHubUpdateSource(id, false)}
	}
	return apps
}

func concurrentChecksRelease() GitHubRelease {
	return GitHubRelease{TagName: "v2.0.0", Assets: []GitHubReleaseAsset{{Name: "Example.AppImage"}}}
}

// Timeouts are deadlock guards; synchronization and assertions use barriers.
func concurrentChecksReceive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for update-check barrier")
		var zero T
		return zero
	}
}

func TestConcurrentUpdateChecksOverlapBoundAndOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	apps := concurrentChecksApps(11)
	started := make(chan string, len(apps))
	gates := make(map[string]chan struct{}, len(apps))
	for _, installed := range apps {
		gates[installed.ID] = make(chan struct{})
	}
	var mu sync.Mutex
	active, maximum, calls := 0, 0, 0
	finder := concurrentChecksFinder(func(ctx context.Context, repo string) (GitHubRelease, error) {
		mu.Lock()
		active++
		calls++
		maximum = max(maximum, active)
		mu.Unlock()
		defer func() { mu.Lock(); active--; mu.Unlock() }()
		started <- repo
		select {
		case <-gates[repo]:
			return concurrentChecksRelease(), nil
		case <-ctx.Done():
			return GitHubRelease{}, ctx.Err()
		}
	})
	s := &service{apps: concurrentChecksRepository{apps: apps}, githubReleases: finder}
	activity := &concurrentChecksActivity{}
	finished := make(chan struct{})
	var plans []githubUpdatePlan
	var candidates []UpdateCandidate
	var failures []UpdateFailure
	var skips []UpdateSkip
	var checked int
	var err error
	go func() {
		plans, candidates, failures, skips, _, checked, err = s.planGitHubUpdates(ctx, "", activity, false)
		close(finished)
	}()
	// All four must start before any can finish, proving actual overlap.
	for offset := 0; offset < len(apps); offset += 4 {
		wave := make([]string, min(4, len(apps)-offset))
		for i := range wave {
			wave[i] = concurrentChecksReceive(t, started)
		}
		for i := len(wave) - 1; i >= 0; i-- {
			close(gates[wave[i]])
		}
	}
	concurrentChecksReceive(t, finished)
	if err != nil || checked != len(apps) || len(failures) != 0 || len(skips) != 0 {
		t.Fatalf("checked=%d failures=%v skips=%v err=%v", checked, failures, skips, err)
	}
	if maximum != 4 || active != 0 || calls != len(apps) {
		t.Fatalf("maximum=%d active=%d calls=%d", maximum, active, calls)
	}
	if len(plans) != len(apps) || len(candidates) != len(apps) {
		t.Fatalf("plans=%d candidates=%d", len(plans), len(candidates))
	}
	for i, installed := range apps {
		want := UpdateCandidate{ID: installed.ID, CurrentVersion: "1.0.0", NewVersion: "2.0.0"}
		if plans[i].app.ID != installed.ID || candidates[i] != want {
			t.Fatalf("index %d: plan=%s candidate=%+v", i, plans[i].app.ID, candidates[i])
		}
	}
	if !reflect.DeepEqual(activity.activities, []Activity{{Kind: ActivityKindCheckingUpdates}}) ||
		!reflect.DeepEqual(activity.done, []string{"Checked integrated apps"}) || len(activity.failed) != 0 {
		t.Fatalf("unexpected activity: %+v", activity)
	}
}

func TestConcurrentUpdateChecksFailureIsolationAndSkips(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	apps := concurrentChecksApps(8)
	apps[1].UpdateSource = domain.UpdateSource{}
	apps[3].UpdateSource = domain.UpdateSource{Kind: domain.UpdateSourceKindZsync}
	apps[5].UpdateSource.Repo = "  "
	apps[7].Version, _ = domain.ParseVersion("2.0.0")
	started := make(chan string, 4)
	gates := make(map[string]chan struct{})
	completed := make(map[string]chan struct{})
	for _, i := range []int{0, 2, 4, 6} {
		gates[apps[i].ID] = make(chan struct{})
		completed[apps[i].ID] = make(chan struct{})
	}
	finder := concurrentChecksFinder(func(ctx context.Context, repo string) (GitHubRelease, error) {
		if gate := gates[repo]; gate != nil {
			defer close(completed[repo])
			started <- repo
			select {
			case <-gate:
			case <-ctx.Done():
				return GitHubRelease{}, ctx.Err()
			}
		}
		switch repo {
		case apps[0].ID:
			return GitHubRelease{}, errors.New("lookup failed")
		case apps[4].ID:
			return GitHubRelease{TagName: "v2.0.0"}, nil
		default:
			return concurrentChecksRelease(), nil
		}
	})
	s := &service{apps: concurrentChecksRepository{apps: apps}, githubReleases: finder}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		plans, candidates, failures, skips, _, checked, err := s.planGitHubUpdates(ctx, "", NoopActivityReporter{}, false)
		if err != nil || checked != 8 {
			t.Errorf("checked=%d err=%v", checked, err)
			return
		}
		if len(plans) != 2 || len(candidates) != 2 || plans[0].app.ID != apps[2].ID || plans[1].app.ID != apps[6].ID || candidates[0].ID != apps[2].ID || candidates[1].ID != apps[6].ID {
			t.Errorf("unexpected plans/candidates: %v / %v", plans, candidates)
		}
		if len(failures) != 2 || failures[0].AppID != apps[0].ID || failures[0].Error != "lookup failed" || failures[1].AppID != apps[4].ID || !strings.Contains(failures[1].Error, "no AppImage assets") {
			t.Errorf("unexpected failures: %v", failures)
		}
		wantSkips := []UpdateSkip{
			{AppID: apps[1].ID, Reason: UpdateSkipReasonNoSource},
			{AppID: apps[3].ID, Reason: UpdateSkipReasonUnsupportedSource, SourceKind: string(domain.UpdateSourceKindZsync)},
			{AppID: apps[5].ID, Reason: UpdateSkipReasonInvalidSource, SourceKind: string(domain.UpdateSourceKindGitHub)},
		}
		if !reflect.DeepEqual(skips, wantSkips) {
			t.Errorf("skips=%v want %v", skips, wantSkips)
		}
	}()
	for range 4 {
		concurrentChecksReceive(t, started)
	}
	// Release later installed apps first, including a failure before a success.
	for _, i := range []int{6, 4, 2, 0} {
		close(gates[apps[i].ID])
		concurrentChecksReceive(t, completed[apps[i].ID])
	}
	concurrentChecksReceive(t, finished)
}

func TestConcurrentUpdateChecksCancellationWaitsAndStopsScheduling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	apps := concurrentChecksApps(12)
	started := make(chan string, len(apps))
	canceled := make(chan struct{}, len(apps))
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseWorkers := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseWorkers()
	finder := concurrentChecksFinder(func(ctx context.Context, repo string) (GitHubRelease, error) {
		started <- repo
		<-ctx.Done()
		canceled <- struct{}{}
		<-release
		return GitHubRelease{}, errors.New("transport canceled")
	})
	s := &service{apps: concurrentChecksRepository{apps: apps}, githubReleases: finder}
	activity := &concurrentChecksActivity{}
	finished := make(chan error, 1)
	go func() {
		plans, candidates, failures, skips, _, checked, err := s.planGitHubUpdates(ctx, "", activity, false)
		if plans != nil || candidates != nil || failures != nil || skips != nil || checked != 0 {
			t.Error("canceled planning returned partial results")
		}
		finished <- err
	}()
	for range 4 {
		concurrentChecksReceive(t, started)
	}
	cancel()
	for range 4 {
		concurrentChecksReceive(t, canceled)
	}
	select {
	case err := <-finished:
		t.Fatalf("returned before workers exited: %v", err)
	default:
	}
	releaseWorkers()
	if err := concurrentChecksReceive(t, finished); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context.Canceled", err)
	}
	if len(started) != 0 {
		t.Fatalf("scheduled %d extra checks after cancellation", len(started))
	}
	if len(activity.done) != 0 || len(activity.failed) != 1 || !errors.Is(activity.failed[0], context.Canceled) {
		t.Fatalf("unexpected activity: %+v", activity)
	}
}

func TestConcurrentUpdateChecksScopeAndErrors(t *testing.T) {
	apps := concurrentChecksApps(2)
	for _, tc := range []struct {
		name          string
		target        string
		repoErr       error
		missingFinder bool
		cancel        bool
		wantErr       error
	}{
		{name: "target", target: apps[1].ID},
		{name: "missing target", target: "missing", wantErr: ErrAppNotFound},
		{name: "repository failure", repoErr: ErrAppNotFound, wantErr: ErrAppNotFound},
		{name: "missing finder", missingFinder: true},
		{name: "already canceled", cancel: true, wantErr: context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			s := &service{apps: concurrentChecksRepository{apps: apps, err: tc.repoErr}}
			if !tc.missingFinder {
				s.githubReleases = concurrentChecksFinder(func(_ context.Context, repo string) (GitHubRelease, error) {
					if repo != apps[1].ID {
						t.Errorf("unexpected repo: %s", repo)
					}
					return concurrentChecksRelease(), nil
				})
			}
			activity := &concurrentChecksActivity{}
			plans, candidates, _, _, _, checked, err := s.planGitHubUpdates(ctx, tc.target, activity, false)
			if tc.missingFinder {
				if err == nil || err.Error() != "github release finder is required" {
					t.Fatalf("err=%v", err)
				}
			} else if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err=%v want %v", err, tc.wantErr)
			}
			if err != nil {
				if len(activity.failed) != 1 || len(activity.done) != 0 {
					t.Fatalf("activity=%+v", activity)
				}
			} else if checked != 1 || len(plans) != 1 || len(candidates) != 1 || candidates[0].ID != apps[1].ID {
				t.Fatalf("checked=%d plans=%v candidates=%v", checked, plans, candidates)
			}
		})
	}
}

func TestConcurrentUpdateChecksNoEligibleApps(t *testing.T) {
	for _, count := range []int{0, 3} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			apps := concurrentChecksApps(count)
			for i := range apps {
				apps[i].UpdateSource = domain.UpdateSource{}
			}
			// A check-only planning pass needs neither a finder nor mutation adapters.
			s := &service{apps: concurrentChecksRepository{apps: apps}}
			activity := &concurrentChecksActivity{}
			plans, candidates, failures, skips, _, checked, err := s.planGitHubUpdates(context.Background(), "", activity, false)
			if err != nil || checked != count || len(plans) != 0 || len(candidates) != 0 || len(failures) != 0 || len(skips) != count {
				t.Fatalf("checked=%d plans=%v candidates=%v failures=%v skips=%v err=%v", checked, plans, candidates, failures, skips, err)
			}
			if plans == nil || candidates == nil || failures == nil || skips == nil {
				t.Fatal("successful planning must preserve non-nil result slices")
			}
			if len(activity.done) != 1 || len(activity.failed) != 0 {
				t.Fatalf("activity=%+v", activity)
			}
		})
	}
}

type concurrentChecksLocker struct {
	locks   int
	unlocks int
}

func (l *concurrentChecksLocker) Lock(context.Context) (func(), error) {
	l.locks++
	return func() { l.unlocks++ }, nil
}

type concurrentChecksConfirmation func(context.Context, []UpdateCandidate) (bool, error)

func (f concurrentChecksConfirmation) ConfirmUpdates(ctx context.Context, updates []UpdateCandidate) (bool, error) {
	return f(ctx, updates)
}

type concurrentChecksMutationSpy struct {
	downloads atomic.Int32
	extracts  atomic.Int32
}

func (s *concurrentChecksMutationSpy) Download(context.Context, DownloadSource, string, DownloadProgress) (DownloadedFile, error) {
	s.downloads.Add(1)
	return DownloadedFile{}, errors.New("unexpected download")
}

func (s *concurrentChecksMutationSpy) Extract(context.Context, string, string) (AppImageExtraction, error) {
	s.extracts.Add(1)
	return AppImageExtraction{}, errors.New("unexpected integration")
}

func TestConcurrentUpdateChecksPublicUpdateCheckOnlyAndDeclined(t *testing.T) {
	for _, checkOnly := range []bool{true, false} {
		name := "declined confirmation"
		if checkOnly {
			name = "check only"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			apps := concurrentChecksApps(4)
			started := make(chan string, len(apps))
			gates := make(map[string]chan struct{}, len(apps))
			completed := make(map[string]chan struct{}, len(apps))
			want := make([]UpdateCandidate, len(apps))
			for i, installed := range apps {
				gates[installed.ID] = make(chan struct{})
				completed[installed.ID] = make(chan struct{})
				want[i] = UpdateCandidate{ID: installed.ID, CurrentVersion: "1.0.0", NewVersion: "2.0.0"}
			}
			finder := concurrentChecksFinder(func(ctx context.Context, repo string) (GitHubRelease, error) {
				defer close(completed[repo])
				started <- repo
				select {
				case <-gates[repo]:
					return concurrentChecksRelease(), nil
				case <-ctx.Done():
					return GitHubRelease{}, ctx.Err()
				}
			})
			confirmations := make(chan []UpdateCandidate, 1)
			confirmation := concurrentChecksConfirmation(func(_ context.Context, updates []UpdateCandidate) (bool, error) {
				for _, installed := range apps {
					select {
					case <-completed[installed.ID]:
					default:
						t.Errorf("confirmation before check completed for %s", installed.ID)
					}
				}
				confirmations <- append([]UpdateCandidate(nil), updates...)
				return false, nil
			})
			locker := &concurrentChecksLocker{}
			mutations := &concurrentChecksMutationSpy{}
			var s Service = &service{
				apps: concurrentChecksRepository{apps: apps}, githubReleases: finder,
				mutations: locker, downloads: mutations, appImages: mutations,
			}
			activity := &concurrentChecksActivity{}
			type outcome struct {
				result UpdateResult
				err    error
			}
			finished := make(chan outcome, 1)
			go func() {
				result, err := s.Update(ctx, UpdateRequest{CheckOnly: checkOnly, Confirmation: confirmation, Activity: activity})
				finished <- outcome{result: result, err: err}
			}()
			// No lookup may finish until all four have started through public Update.
			for range apps {
				concurrentChecksReceive(t, started)
			}
			if len(confirmations) != 0 {
				t.Fatal("confirmation called while checks were blocked")
			}
			// Complete lookups in reverse installed order to exercise ordered output.
			for i := len(apps) - 1; i >= 0; i-- {
				close(gates[apps[i].ID])
				concurrentChecksReceive(t, completed[apps[i].ID])
			}
			got := concurrentChecksReceive(t, finished)
			if got.err != nil || got.result.Applied || got.result.AppliedCount != 0 || got.result.Checked != len(apps) ||
				len(got.result.Failures) != 0 || len(got.result.Skipped) != 0 || !reflect.DeepEqual(got.result.Updates, want) {
				t.Fatalf("Update() = %+v, %v", got.result, got.err)
			}
			if checkOnly {
				if len(confirmations) != 0 {
					t.Fatal("check-only update requested confirmation")
				}
			} else if updates := concurrentChecksReceive(t, confirmations); !reflect.DeepEqual(updates, want) {
				t.Fatalf("confirmation updates = %v, want %v", updates, want)
			}
			if mutations.downloads.Load() != 0 || mutations.extracts.Load() != 0 {
				t.Fatalf("downloads=%d integrations=%d", mutations.downloads.Load(), mutations.extracts.Load())
			}
			if locker.locks != 1 || locker.unlocks != 1 {
				t.Fatalf("locks=%d unlocks=%d", locker.locks, locker.unlocks)
			}
			if !reflect.DeepEqual(activity.activities, []Activity{{Kind: ActivityKindCheckingUpdates}}) ||
				!reflect.DeepEqual(activity.done, []string{"Checked integrated apps"}) || len(activity.failed) != 0 {
				t.Fatalf("unexpected activity: %+v", activity)
			}
		})
	}
}
