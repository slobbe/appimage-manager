package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"sync/atomic"
	"testing"
	"time"

	"github.com/slobbe/appimage-manager/internal/domain"
)

type stagedDownloadCall struct {
	name string
	path string
}

type stagedDownloader struct {
	started       chan stagedDownloadCall
	settled       chan string
	canceled      chan string
	gates         map[string]chan struct{}
	failures      map[string]error
	finished      atomic.Int32
	active        atomic.Int32
	exceededLimit atomic.Bool
}

func (d *stagedDownloader) Download(ctx context.Context, source DownloadSource, path string, _ DownloadProgress) (DownloadedFile, error) {
	if d.active.Add(1) > 3 {
		d.exceededLimit.Store(true)
	}
	defer d.active.Add(-1)
	d.started <- stagedDownloadCall{name: source.URL, path: path}
	select {
	case <-d.gates[source.URL]:
	case <-ctx.Done():
		d.canceled <- source.URL
		// Deliberately delay worker exit to verify Update joins workers before cleanup.
		<-d.gates[source.URL]
	}
	defer func() {
		d.finished.Add(1)
		d.settled <- source.URL
	}()
	if err := ctx.Err(); err != nil {
		return DownloadedFile{}, err
	}
	if err := d.failures[source.URL]; err != nil {
		return DownloadedFile{}, err
	}
	if err := os.WriteFile(path, []byte("appimage"), 0o600); err != nil {
		return DownloadedFile{}, err
	}
	return DownloadedFile{}, nil // Exercise the destination-path fallback.
}

type stagedReleases struct{}

func (stagedReleases) LatestRelease(_ context.Context, repo string, _ bool) (GitHubRelease, error) {
	release := testGitHubReleaseWithTag("v2.0.0", "Example.AppImage")
	release.Assets[0].DownloadURL = repo
	return release, nil
}

func (r stagedReleases) LatestReleaseMatchingTag(ctx context.Context, repo string, includePrerelease bool, _ string) (GitHubRelease, error) {
	return r.LatestRelease(ctx, repo, includePrerelease)
}

func (r stagedReleases) LatestPrerelease(ctx context.Context, repo string) (GitHubRelease, error) {
	return r.LatestRelease(ctx, repo, true)
}

func (r stagedReleases) ReleaseByTag(ctx context.Context, repo, _ string) (GitHubRelease, error) {
	return r.LatestRelease(ctx, repo, false)
}

type stagedInstaller struct {
	AppImageInstaller
	downloads  *stagedDownloader
	total      int32
	entered    chan string
	release    chan struct{}
	active     atomic.Int32
	overlapped atomic.Bool
	early      atomic.Bool
}

func (i *stagedInstaller) Install(ctx context.Context, path, id string) (string, error) {
	if i.active.Add(1) != 1 {
		i.overlapped.Store(true)
	}
	defer i.active.Add(-1)
	if i.downloads.finished.Load() != i.total {
		i.early.Store(true)
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("download disappeared before integration: %w", err)
	}
	i.entered <- id
	select {
	case <-i.release:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return i.AppImageInstaller.Install(ctx, path, id)
}

type stagedMutationLocker struct{ held atomic.Bool }

func (l *stagedMutationLocker) Lock(context.Context) (func(), error) {
	l.held.Store(true)
	return func() { l.held.Store(false) }, nil
}

type stagedUpdateOutcome struct {
	result UpdateResult
	err    error
}

func stagedReceive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for stage barrier")
		var zero T
		return zero
	}
}

func stagedUpdateFixture(t *testing.T, count int) (integrationFakes, *stagedDownloader, *stagedInstaller, *stagedMutationLocker) {
	t.Helper()
	deps := integrationTestDeps()
	downloads := &stagedDownloader{
		started: make(chan stagedDownloadCall, count), settled: make(chan string, count),
		canceled: make(chan string, count), gates: make(map[string]chan struct{}), failures: make(map[string]error),
	}
	deps.appImageInstaller.paths = make(map[string]string)
	deps.iconInstaller.paths = make(map[string]string)
	deps.desktopEntryInstaller.paths = make(map[string]string)
	for n := 0; n < count; n++ {
		id := fmt.Sprintf("app-%d", n)
		installed := testInstalledApp(t)
		installed.ID = id
		installed.AppImagePath = "/library/" + id + ".AppImage"
		installed.IconPath = "/icons/" + id + ".png"
		installed.DesktopEntryPath = "/desktop/" + id + ".desktop"
		installed.UpdateSource = domain.NewGitHubUpdateSource("owner/"+id, false)
		deps.apps.listApps = append(deps.apps.listApps, installed)
		deps.appImageInstaller.paths[id] = installed.AppImagePath
		deps.iconInstaller.paths[id] = installed.IconPath
		deps.desktopEntryInstaller.paths[id] = installed.DesktopEntryPath
		downloads.gates["owner/"+id] = make(chan struct{})
	}
	installer := &stagedInstaller{
		AppImageInstaller: deps.appImageInstaller, downloads: downloads, total: int32(count),
		entered: make(chan string, count), release: make(chan struct{}, count),
	}
	locker := &stagedMutationLocker{}
	deps.ServiceDeps.GitHubReleases = stagedReleases{}
	deps.ServiceDeps.Downloads = downloads
	deps.ServiceDeps.AppImageInstaller = installer
	deps.ServiceDeps.Mutations = locker
	return deps, downloads, installer, locker
}

func stagedStartUpdate(t *testing.T, deps ServiceDeps) (context.CancelFunc, <-chan stagedUpdateOutcome) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	svc, err := NewService(deps)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan stagedUpdateOutcome, 1)
	go func() {
		result, err := svc.Update(ctx, UpdateRequest{})
		done <- stagedUpdateOutcome{result, err}
	}()
	return cancel, done
}

func stagedAssertCleaned(t *testing.T, calls []stagedDownloadCall) {
	t.Helper()
	for _, call := range calls {
		workspace := filepath.Dir(call.path)
		if _, err := os.Stat(workspace); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("workspace %q still exists: %v", workspace, err)
		}
	}
}

func TestStagedUpdateDownloadsOverlapLimitAndSerialIntegration(t *testing.T) {
	deps, downloads, installer, locker := stagedUpdateFixture(t, 6)
	_, done := stagedStartUpdate(t, deps.ServiceDeps)
	var calls []stagedDownloadCall
	// Three downloads must enter without any being released: actual overlap,
	// with the fourth held back by the worker limit.
	for n := 0; n < 3; n++ {
		calls = append(calls, stagedReceive(t, downloads.started))
	}
	select {
	case call := <-downloads.started:
		t.Fatalf("fourth download started before a worker settled: %#v", call)
	default:
	}
	if !locker.held.Load() {
		t.Fatal("mutation lock not held during downloads")
	}
	// Complete later plans first, leaving the first two blocked while all other
	// plans run. No integration may start until those final downloads settle.
	for n := 2; n < 6; n++ {
		close(downloads.gates[calls[n].name])
		stagedReceive(t, downloads.settled)
		if n < 5 {
			calls = append(calls, stagedReceive(t, downloads.started))
		}
	}
	select {
	case id := <-installer.entered:
		t.Fatalf("integration %s began during downloads", id)
	default:
	}
	close(downloads.gates[calls[1].name])
	stagedReceive(t, downloads.settled)
	for _, call := range calls {
		if _, err := os.Stat(filepath.Dir(call.path)); err != nil {
			t.Fatalf("workspace removed before integration: %v", err)
		}
	}
	close(downloads.gates[calls[0].name])
	for n := 0; n < 6; n++ {
		id := stagedReceive(t, installer.entered)
		if want := fmt.Sprintf("app-%d", n); id != want {
			t.Fatalf("integration order: got %s, want %s", id, want)
		}
		if !locker.held.Load() {
			t.Fatal("mutation lock not held during integration")
		}
		select {
		case next := <-installer.entered:
			t.Fatalf("integration %s overlapped %s", next, id)
		default:
		}
		installer.release <- struct{}{}
	}
	outcome := stagedReceive(t, done)
	if outcome.err != nil || outcome.result.AppliedCount != 6 || len(outcome.result.Failures) != 0 {
		t.Fatalf("Update() = %#v, %v", outcome.result, outcome.err)
	}
	if downloads.exceededLimit.Load() {
		t.Fatal("more than three downloads ran concurrently")
	}
	if installer.early.Load() || installer.overlapped.Load() {
		t.Fatal("integration violated the download boundary or serial execution")
	}
	if locker.held.Load() {
		t.Fatal("mutation lock not released")
	}
	stagedAssertCleaned(t, calls)
}

type stagedFailingExtractor struct {
	AppImageExtractor
	calls atomic.Int32
}

func (e *stagedFailingExtractor) Extract(ctx context.Context, path, dest string) (AppImageExtraction, error) {
	if e.calls.Add(1) == 1 {
		return AppImageExtraction{}, errors.New("extraction failed")
	}
	return e.AppImageExtractor.Extract(ctx, path, dest)
}

func TestStagedUpdateDownloadsFailureIsIndependentAndCleansWorkspaces(t *testing.T) {
	deps, downloads, installer, _ := stagedUpdateFixture(t, 3)
	downloads.failures["owner/app-1"] = errors.New("download failed")
	_, done := stagedStartUpdate(t, deps.ServiceDeps)
	var calls []stagedDownloadCall
	for n := 0; n < 3; n++ {
		calls = append(calls, stagedReceive(t, downloads.started))
	}
	// Release in reverse arrival order, including the failed download.
	for n := 2; n >= 0; n-- {
		close(downloads.gates[calls[n].name])
		stagedReceive(t, downloads.settled)
	}
	for _, want := range []string{"app-0", "app-2"} {
		if got := stagedReceive(t, installer.entered); got != want {
			t.Fatalf("integrated %s, want %s", got, want)
		}
		installer.release <- struct{}{}
	}
	outcome := stagedReceive(t, done)
	if outcome.err != nil || outcome.result.AppliedCount != 2 || !outcome.result.Applied {
		t.Fatalf("Update() = %#v, %v", outcome.result, outcome.err)
	}
	if want := []UpdateFailure{{AppID: "app-1", Error: "download failed"}}; !reflect.DeepEqual(outcome.result.Failures, want) {
		t.Fatalf("failures = %#v, want %#v", outcome.result.Failures, want)
	}
	if installer.early.Load() || installer.overlapped.Load() {
		t.Fatal("integration violated stage ordering")
	}
	stagedAssertCleaned(t, calls)
}

func TestStagedUpdateDownloadsCancellationJoinsWorkersAndLeavesInstallsUntouched(t *testing.T) {
	deps, downloads, installer, locker := stagedUpdateFixture(t, 7)
	cancel, done := stagedStartUpdate(t, deps.ServiceDeps)
	var calls []stagedDownloadCall
	for n := 0; n < 3; n++ {
		calls = append(calls, stagedReceive(t, downloads.started))
	}
	// Include a successfully downloaded workspace in the cancellation cleanup.
	close(downloads.gates[calls[0].name])
	stagedReceive(t, downloads.settled)
	calls = append(calls, stagedReceive(t, downloads.started))
	cancel()
	for n := 0; n < 3; n++ {
		stagedReceive(t, downloads.canceled)
	}
	select {
	case outcome := <-done:
		t.Fatalf("Update returned before workers settled: %#v", outcome)
	default:
	}
	if !locker.held.Load() {
		t.Fatal("mutation lock released before workers settled")
	}
	for _, call := range calls {
		if _, err := os.Stat(filepath.Dir(call.path)); err != nil {
			t.Fatalf("workspace removed while workers still active: %v", err)
		}
	}
	for _, call := range calls[1:] {
		close(downloads.gates[call.name])
	}
	outcome := stagedReceive(t, done)
	if !errors.Is(outcome.err, context.Canceled) {
		t.Fatalf("Update() error = %v, want cancellation", outcome.err)
	}
	select {
	case call := <-downloads.started:
		t.Fatalf("download scheduled after cancellation: %#v", call)
	default:
	}
	select {
	case id := <-installer.entered:
		t.Fatalf("integration %s began after canceled download stage", id)
	default:
	}
	if deps.appImageInstaller.called || deps.saved.App.ID != "" || len(deps.artifactBackups.paths) != 0 {
		t.Fatal("canceled download stage changed installed artifacts or state")
	}
	if locker.held.Load() {
		t.Fatal("mutation lock not released")
	}
	stagedAssertCleaned(t, calls)
}

func TestStagedUpdateDownloadsIntegrationFailureCleansAndContinues(t *testing.T) {
	deps, downloads, installer, _ := stagedUpdateFixture(t, 2)
	deps.ServiceDeps.AppImages = &stagedFailingExtractor{AppImageExtractor: deps.appImages}
	_, done := stagedStartUpdate(t, deps.ServiceDeps)
	var calls []stagedDownloadCall
	for n := 0; n < 2; n++ {
		calls = append(calls, stagedReceive(t, downloads.started))
	}
	for _, call := range calls {
		close(downloads.gates[call.name])
	}
	if got := stagedReceive(t, installer.entered); got != "app-1" {
		t.Fatalf("integrated %s, want app-1", got)
	}
	installer.release <- struct{}{}
	outcome := stagedReceive(t, done)
	if outcome.err != nil || outcome.result.AppliedCount != 1 {
		t.Fatalf("Update() = %#v, %v", outcome.result, outcome.err)
	}
	if want := []UpdateFailure{{AppID: "app-0", Error: "extraction failed"}}; !reflect.DeepEqual(outcome.result.Failures, want) {
		t.Fatalf("failures = %#v, want %#v", outcome.result.Failures, want)
	}
	stagedAssertCleaned(t, calls)
}
