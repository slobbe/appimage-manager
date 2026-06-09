package services

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/slobbe/appimage-manager/internal/app/discovery"
	appselfupdate "github.com/slobbe/appimage-manager/internal/app/selfupdate"
	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	"github.com/slobbe/appimage-manager/internal/domain"
	"github.com/slobbe/appimage-manager/internal/infra/download"
)

func TestDefaultManagedUpdateServiceUsesDownloadHTTPClient(t *testing.T) {
	originalDownloadClient := download.SharedHTTPClient()
	originalDownloadTimeout := download.SharedHTTPClient().Timeout
	t.Cleanup(func() {
		download.SetHTTPClientTimeout(originalDownloadTimeout)
		if originalDownloadClient != nil && originalDownloadClient.Transport != nil {
			download.SharedHTTPClient().Transport = originalDownloadClient.Transport
		}
	})
	download.SetHTTPClientTimeout(75 * time.Millisecond)

	apiClient := &http.Client{Timeout: 75 * time.Millisecond}
	service := NewDefaultManagedUpdateService(RuntimePaths{TempDir: t.TempDir()}, apiClient, func() string { return "now" }, nil)

	staged, ok := service.StagedDownload.(defaultStagedDownloadAdapter)
	if !ok {
		t.Fatalf("staged download adapter = %T, want defaultStagedDownloadAdapter", service.StagedDownload)
	}
	if staged.client != download.SharedHTTPClient() {
		t.Fatal("managed update staged download should use shared download HTTP client")
	}
	if staged.client == apiClient {
		t.Fatal("managed update staged download should not use API HTTP client with whole-body timeout")
	}
	if got := staged.client.Timeout; got != 0 {
		t.Fatalf("managed update download client timeout = %s, want 0", got)
	}
}

func TestStoreListServiceFiltersApps(t *testing.T) {
	store := fakeAppStore{apps: map[string]*domain.App{
		"integrated": {ID: "integrated", DesktopEntryLink: "/tmp/integrated.desktop"},
		"unlinked":   {ID: "unlinked"},
	}}

	result, err := (StoreListService{Store: store}).List(context.Background(), ListRequest{Filter: ListIntegrated})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(result.Apps) != 1 || result.Apps[0].ID != "integrated" || result.Apps[0].DesktopEntryLink == "" {
		t.Fatalf("List returned %+v, want integrated app only", result.Apps)
	}
	if result.TotalCount != 2 || result.IntegratedCount != 1 || result.UnlinkedCount != 1 {
		t.Fatalf("List counts = total %d integrated %d unlinked %d, want 2/1/1", result.TotalCount, result.IntegratedCount, result.UnlinkedCount)
	}
}

func TestStoreInfoServiceRoutesInfoRequests(t *testing.T) {
	store := fakeAppStore{apps: map[string]*domain.App{
		"managed": {ID: "managed", Name: "Managed App", ExecPath: "/apps/managed.AppImage"},
	}}
	service := StoreInfoService{
		Store: store,
		AppImage: AppImageInfoReaderFunc(func(ctx context.Context, path string) (*domain.AppInfo, error) {
			_ = ctx
			return &domain.AppInfo{ID: "local", Name: "Local App", Version: "1.0.0"}, nil
		}),
		Discovery: DiscoveryWorkflowService{Backends: []discovery.DiscoveryBackend{
			fakeDiscoveryBackend{metadata: &domain.PackageMetadata{Name: "Package App", Ref: domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: "owner/repo"}}},
		}},
	}

	managed, err := service.Info(context.Background(), InfoRequest{Input: "managed"})
	if err != nil {
		t.Fatalf("managed Info returned error: %v", err)
	}
	if managed.Kind != InfoKindManagedApp || managed.App == nil || managed.App.ID != "managed" {
		t.Fatalf("managed Info = %+v", managed)
	}

	local, err := service.Info(context.Background(), InfoRequest{Input: "/tmp/App.AppImage"})
	if err != nil {
		t.Fatalf("local Info returned error: %v", err)
	}
	if local.Kind != InfoKindLocalAppImage || local.AppImageInfo == nil || local.AppImageInfo.ID != "local" {
		t.Fatalf("local Info = %+v", local)
	}

	provider := domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: "owner/repo"}
	pkg, err := service.Info(context.Background(), InfoRequest{Provider: &provider})
	if err != nil {
		t.Fatalf("package Info returned error: %v", err)
	}
	if pkg.Kind != InfoKindPackage || pkg.PackageMetadata == nil || pkg.PackageMetadata.Name != "Package App" {
		t.Fatalf("package Info = %+v", pkg)
	}
}

func TestStoreInfoServiceManagedOnlyTreatsAppImageLikeInputAsID(t *testing.T) {
	service := StoreInfoService{Store: fakeAppStore{apps: map[string]*domain.App{
		"managed.AppImage": {ID: "managed.AppImage", Name: "Managed App"},
	}}}

	result, err := service.Info(context.Background(), InfoRequest{Input: "managed.AppImage", ManagedOnly: true})
	if err != nil {
		t.Fatalf("Info returned error: %v", err)
	}
	if result.Kind != InfoKindManagedApp || result.App == nil || result.App.ID != "managed.AppImage" {
		t.Fatalf("Info result = %+v", result)
	}
}

func TestAddWorkflowServiceInstallsDirectURLViaInstaller(t *testing.T) {
	installer := &fakeRemoteInstaller{}
	service := AddWorkflowService{Installer: installer}

	result, err := service.Add(context.Background(), AddRequest{
		Target: AddTargetInput{URL: "https://example.com/App.AppImage"},
		SHA256: "abc123",
	})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if installer.directCalls != 1 || installer.directReq.URL != "https://example.com/App.AppImage" || installer.directReq.SHA256 != "abc123" {
		t.Fatalf("direct install request = %+v calls = %d", installer.directReq, installer.directCalls)
	}
	if result == nil || result.Action != AddActionInstall || result.Status != "installed" || result.App == nil || result.App.ID != "direct" {
		t.Fatalf("Add result = %+v", result)
	}
}

func TestAddWorkflowServiceInstallsPackageRefViaDiscoveryAndInstaller(t *testing.T) {
	metadata := &domain.PackageMetadata{
		Name:        "Package App",
		Ref:         domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: "owner/repo"},
		Installable: true,
		DownloadURL: "https://example.com/old.AppImage",
		AssetName:   "old.AppImage",
	}
	installer := &fakeRemoteInstaller{}
	service := AddWorkflowService{
		Discovery: DiscoveryWorkflowService{Backends: []discovery.DiscoveryBackend{fakeDiscoveryBackend{metadata: metadata}}},
		Installer: installer,
	}
	provider := domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: "owner/repo"}

	result, err := service.Add(context.Background(), AddRequest{
		Target:       AddTargetInput{Provider: &provider},
		AssetPattern: "*.AppImage",
		ResolvePackageAmbiguity: packageAmbiguityResolverFunc(func(view *domain.PackageMetadata) (*domain.PackageMetadata, error) {
			view.AssetName = "selected.AppImage"
			view.DownloadURL = "https://example.com/selected.AppImage"
			return view, nil
		}),
	})
	if err != nil {
		t.Fatalf("Add returned error: %v", err)
	}
	if installer.packageCalls != 1 || installer.packageMetadata == nil {
		t.Fatalf("package install calls = %d metadata = %+v", installer.packageCalls, installer.packageMetadata)
	}
	if installer.packageMetadata.AssetName != "selected.AppImage" || installer.packageMetadata.DownloadURL != "https://example.com/selected.AppImage" {
		t.Fatalf("package metadata selection = %+v", installer.packageMetadata)
	}
	if result == nil || result.Action != AddActionInstall || result.Status != "installed" || result.App == nil || result.App.ID != "package" {
		t.Fatalf("Add result = %+v", result)
	}
}

func TestSourceUpdateServiceUpdateDryRunMarksPendingWithoutApplying(t *testing.T) {
	store := fakeAppStore{apps: map[string]*domain.App{"app": {ID: "app", Name: "App", Version: "1.0.0"}}}
	var applyCalls int
	service := SourceUpdateService{
		Store: store,
		CheckManagedUpdate: func(app *domain.App) (*appupdate.ManagedUpdate, error) {
			return &appupdate.ManagedUpdate{App: app, Available: true, Latest: "2.0.0", URL: "https://example.com/App.AppImage"}, nil
		},
		ApplyManagedUpdate: func(context.Context, appupdate.ManagedUpdate, appupdate.ManagedApplyReporter) (*domain.App, error) {
			applyCalls++
			return nil, nil
		},
	}

	result, err := service.Update(context.Background(), UpdateRequest{DryRun: true})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if applyCalls != 0 {
		t.Fatalf("apply calls = %d, want 0", applyCalls)
	}
	if result == nil || len(result.Rows) != 1 || result.Rows[0].Status != "dry_run_pending" || len(result.Pending) != 1 {
		t.Fatalf("Update result = %+v", result)
	}
}

func TestSourceUpdateServiceUpdateAppliesWithLockerAndReconcilesRows(t *testing.T) {
	store := fakeAppStore{apps: map[string]*domain.App{"app": {ID: "app", Name: "App", Version: "1.0.0"}}}
	locker := &fakeStateLocker{}
	service := SourceUpdateService{
		Store:  store,
		Locker: locker,
		CheckManagedUpdate: func(app *domain.App) (*appupdate.ManagedUpdate, error) {
			return &appupdate.ManagedUpdate{App: app, Available: true, Latest: "2.0.0", URL: "https://example.com/App.AppImage"}, nil
		},
		ApplyManagedUpdate: func(ctx context.Context, update appupdate.ManagedUpdate, reporter appupdate.ManagedApplyReporter) (*domain.App, error) {
			_ = ctx
			_ = update
			_ = reporter
			return &domain.App{ID: "app", Name: "App", Version: "2.0.0"}, nil
		},
		PersistApp: func(*domain.App, bool) error { return nil },
	}

	result, err := service.Update(context.Background(), UpdateRequest{AutoApply: true})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if !locker.called {
		t.Fatal("expected update apply to use locker")
	}
	if result == nil || len(result.Applied) != 1 || len(result.Rows) != 1 || result.Rows[0].Status != "updated" || result.Rows[0].App.Version != "2.0.0" {
		t.Fatalf("Update result = %+v", result)
	}
}

func TestSourceUpdateServiceSetAndPlan(t *testing.T) {
	store := fakeAppStore{apps: map[string]*domain.App{
		"app": {ID: "app", Update: &domain.UpdateSource{Kind: domain.UpdateNone}},
	}}
	source := &domain.UpdateSource{
		Kind: domain.UpdateGitHubRelease,
		GitHubRelease: &domain.GitHubReleaseUpdateSource{
			Repo:  "owner/repo",
			Asset: "*.AppImage",
		},
	}
	service := SourceUpdateService{Store: store}

	planResult, err := service.Update(context.Background(), UpdateRequest{TargetID: "app", Mode: UpdateModeSetSource, Source: source, DryRun: true})
	if err != nil {
		t.Fatalf("Update dry-run set source returned error: %v", err)
	}
	if planResult.Plan == nil || planResult.Plan.Action != "set_update_source" {
		t.Fatalf("unexpected plan result: %+v", planResult)
	}
	if planResult.SourceChange == nil || planResult.SourceChange.Incoming == nil || planResult.SourceChange.Incoming.GitHubRelease == nil {
		t.Fatalf("plan missing typed update source change: %+v", planResult.SourceChange)
	}

	result, err := service.Update(context.Background(), UpdateRequest{TargetID: "app", Mode: UpdateModeSetSource, Source: source})
	if err != nil {
		t.Fatalf("Update set source returned error: %v", err)
	}
	if result.Source == nil || !result.Source.Changed || result.Source.Source == nil || result.Source.Source.GitHubRelease == nil || store.apps["app"].Update == nil || store.apps["app"].Update.GitHubRelease == nil {
		t.Fatalf("Update result = %+v app = %+v", result, store.apps["app"])
	}
}

func TestSourceUpdateServicePersistAppliedAppsUsesBatch(t *testing.T) {
	var batchCalls int
	var singleCalls int
	service := SourceUpdateService{
		PersistApps: func(apps []*domain.App, overwrite bool) error {
			batchCalls++
			if !overwrite {
				t.Fatalf("expected overwrite true")
			}
			if len(apps) != 2 {
				t.Fatalf("len(apps) = %d, want 2", len(apps))
			}
			return nil
		},
		PersistApp: func(*domain.App, bool) error {
			singleCalls++
			return nil
		},
		RemoveApp: func(context.Context, string, bool) (*domain.App, error) {
			t.Fatal("RemoveApp should not be called without replacements")
			return nil, nil
		},
	}

	if err := service.persistAppliedApps(context.Background(), []*domain.App{{ID: "a"}, {ID: "b"}}); err != nil {
		t.Fatalf("persistAppliedApps returned error: %v", err)
	}
	if batchCalls != 1 {
		t.Fatalf("batch calls = %d, want 1", batchCalls)
	}
	if singleCalls != 0 {
		t.Fatalf("single calls = %d, want 0", singleCalls)
	}
}

func TestSourceUpdateServicePersistAppliedAppsFallsBackToSingleWrites(t *testing.T) {
	var singleCalls int
	service := SourceUpdateService{
		PersistApps: func([]*domain.App, bool) error {
			return fmt.Errorf("batch failed")
		},
		PersistApp: func(*domain.App, bool) error {
			singleCalls++
			return nil
		},
		RemoveApp: func(context.Context, string, bool) (*domain.App, error) {
			t.Fatal("RemoveApp should not be called without replacements")
			return nil, nil
		},
	}

	if err := service.persistAppliedApps(context.Background(), []*domain.App{{ID: "a"}, {ID: "b"}}); err != nil {
		t.Fatalf("persistAppliedApps returned error: %v", err)
	}
	if singleCalls != 2 {
		t.Fatalf("single calls = %d, want 2", singleCalls)
	}
}

func TestSourceUpdateServicePersistAppliedAppsRemovesSupersededApps(t *testing.T) {
	removed := make([]string, 0, 1)
	service := SourceUpdateService{
		PersistApps: func([]*domain.App, bool) error {
			return nil
		},
		PersistApp: func(*domain.App, bool) error {
			t.Fatal("single fallback should not be used")
			return nil
		},
		RemoveApp: func(_ context.Context, id string, unlink bool) (*domain.App, error) {
			if unlink {
				t.Fatal("expected full removal for superseded app")
			}
			removed = append(removed, id)
			return &domain.App{ID: id}, nil
		},
	}

	if err := service.persistAppliedApps(context.Background(), []*domain.App{
		{ID: "t3-code", ReplacesID: "t3-code-desktop"},
		{ID: "other"},
	}); err != nil {
		t.Fatalf("persistAppliedApps returned error: %v", err)
	}
	if strings.Join(removed, ",") != "t3-code-desktop" {
		t.Fatalf("removed ids = %q, want %q", strings.Join(removed, ","), "t3-code-desktop")
	}
}

func TestSelfUpdateWorkflowServiceSkipsInstallerWhenUpToDate(t *testing.T) {
	var installerCalls int
	service := SelfUpdateWorkflowService{
		CheckFunc: func(context.Context, string, bool) (*AimSelfUpdateCheckResult, error) {
			return &AimSelfUpdateCheckResult{CurrentVersion: "1.0.0", LatestVersion: "1.0.0", Comparable: true, HasUpdate: false}, nil
		},
		SelfUpdateFunc: func(context.Context, appselfupdate.InstallerSelfUpdateRequest) (*InstallerSelfUpdateResult, error) {
			installerCalls++
			return nil, nil
		},
	}

	result, err := service.SelfUpdate(context.Background(), SelfUpdateRequest{CurrentVersion: "1.0.0"})
	if err != nil {
		t.Fatalf("SelfUpdate returned error: %v", err)
	}
	if installerCalls != 0 {
		t.Fatalf("installer calls = %d, want 0", installerCalls)
	}
	if result == nil || !result.UpToDate || result.Updated {
		t.Fatalf("SelfUpdate result = %+v, want up-to-date without self-update", result)
	}
}

func TestSelfUpdateWorkflowServiceReportsCurrentAheadWithoutRunningInstaller(t *testing.T) {
	var installerCalls int
	service := SelfUpdateWorkflowService{
		CheckFunc: func(context.Context, string, bool) (*AimSelfUpdateCheckResult, error) {
			return &AimSelfUpdateCheckResult{CurrentVersion: "0.17.1-pre.4", LatestVersion: "v0.17.0", Comparable: true, HasUpdate: false, CurrentAhead: true}, nil
		},
		SelfUpdateFunc: func(context.Context, appselfupdate.InstallerSelfUpdateRequest) (*InstallerSelfUpdateResult, error) {
			installerCalls++
			return nil, nil
		},
	}

	result, err := service.SelfUpdate(context.Background(), SelfUpdateRequest{CurrentVersion: "0.17.1-pre.4"})
	if err != nil {
		t.Fatalf("SelfUpdate returned error: %v", err)
	}
	if installerCalls != 0 {
		t.Fatalf("installer calls = %d, want 0", installerCalls)
	}
	if result == nil || !result.CurrentAhead || result.UpToDate || result.Updated {
		t.Fatalf("SelfUpdate result = %+v, want current ahead without update", result)
	}
}

func TestSelfUpdateWorkflowServiceRunsInstallerWhenUpdateAvailable(t *testing.T) {
	var gotPreRelease bool
	service := SelfUpdateWorkflowService{
		CheckFunc: func(_ context.Context, _ string, preRelease bool) (*AimSelfUpdateCheckResult, error) {
			gotPreRelease = preRelease
			return &AimSelfUpdateCheckResult{CurrentVersion: "1.0.0", LatestVersion: "1.1.0", Comparable: true, HasUpdate: true}, nil
		},
		SelfUpdateFunc: func(_ context.Context, req appselfupdate.InstallerSelfUpdateRequest) (*InstallerSelfUpdateResult, error) {
			if req.CurrentVersion != "1.0.0" || req.TargetVersion != "1.1.0" {
				t.Fatalf("installer request = %+v", req)
			}
			return &InstallerSelfUpdateResult{PreviousVersion: "1.0.0", InstalledVersion: "1.1.0"}, nil
		},
	}

	result, err := service.SelfUpdate(context.Background(), SelfUpdateRequest{CurrentVersion: "1.0.0", PreRelease: true})
	if err != nil {
		t.Fatalf("SelfUpdate returned error: %v", err)
	}
	if !gotPreRelease {
		t.Fatal("CheckFunc preRelease = false, want true")
	}
	if result == nil || !result.Updated || result.UpToDate || result.CurrentVersion != "1.0.0" || result.LatestVersion != "1.1.0" || result.InstalledVersion != "1.1.0" {
		t.Fatalf("SelfUpdate result = %+v", result)
	}
}

func TestDiscoveryWorkflowServiceUsesConfiguredBackends(t *testing.T) {
	metadata := &domain.PackageMetadata{Name: "App"}
	service := DiscoveryWorkflowService{Backends: []discovery.DiscoveryBackend{
		fakeDiscoveryBackend{metadata: metadata},
	}}

	got, err := service.ResolvePackage(context.Background(), PackageRefInfoRequest{
		Ref: domain.PackageRef{Kind: domain.ProviderGitHub, ProviderRef: "owner/repo"},
	})
	if err != nil {
		t.Fatalf("ResolvePackage returned error: %v", err)
	}
	if got != metadata {
		t.Fatalf("ResolvePackage = %+v, want configured metadata", got)
	}
}

func TestRemoveWorkflowServiceDryRunUsesStoreWithoutRemoving(t *testing.T) {
	var removeCalls int
	service := RemoveWorkflowService{
		Store: fakeAppStore{apps: map[string]*domain.App{
			"app": {ID: "app", Name: "App", ExecPath: "/apps/app/app.AppImage", DesktopEntryLink: "/applications/app.desktop"},
		}},
		RemoveFunc: func(context.Context, string, bool) (*domain.App, error) {
			removeCalls++
			return nil, nil
		},
	}

	result, err := service.Remove(context.Background(), RemoveRequest{ID: "app", DryRun: true})
	if err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if removeCalls != 0 {
		t.Fatalf("remove calls = %d, want 0", removeCalls)
	}
	if result == nil || result.Action != "remove" || result.App == nil || result.App.ID != "app" || len(result.Paths) == 0 {
		t.Fatalf("Remove dry-run result = %+v", result)
	}
}

func TestRemoveWorkflowServiceRemoveUsesRemoveFunc(t *testing.T) {
	var gotID string
	var gotUnlink bool
	service := RemoveWorkflowService{
		RemoveFunc: func(_ context.Context, id string, unlink bool) (*domain.App, error) {
			gotID = id
			gotUnlink = unlink
			return &domain.App{ID: id, Name: "App"}, nil
		},
	}

	result, err := service.Remove(context.Background(), RemoveRequest{ID: "app", Unlink: true})
	if err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if gotID != "app" || !gotUnlink {
		t.Fatalf("RemoveFunc args = id %q unlink %v, want app/true", gotID, gotUnlink)
	}
	if result == nil || result.Action != "unlink" || result.App == nil || result.App.ID != "app" {
		t.Fatalf("Remove result = %+v", result)
	}
}

func TestRemoveDryRunValues(t *testing.T) {
	app := &domain.App{
		ID:               "app",
		ExecPath:         "/apps/app/app.AppImage",
		DesktopEntryLink: "/applications/app.desktop",
		IconPath:         "/icons/app.svg",
	}

	values := RemoveDryRunValues(app, false)
	paths := values["paths"].([]string)
	if values["action"] != "remove" || len(paths) != 3 {
		t.Fatalf("RemoveDryRunValues = %+v", values)
	}
}

type fakeAppStore struct {
	apps map[string]*domain.App
}

func (store fakeAppStore) GetApp(id string) (*domain.App, error) {
	return store.apps[id], nil
}

func (store fakeAppStore) GetAllApps() (map[string]*domain.App, error) {
	return store.apps, nil
}

func (store fakeAppStore) UpdateApp(app *domain.App) error {
	store.apps[app.ID] = app
	return nil
}

type fakeStateLocker struct {
	called bool
}

func (locker *fakeStateLocker) WithWriteLock(fn func() error) error {
	locker.called = true
	return fn()
}

type fakeRemoteInstaller struct {
	directCalls     int
	directReq       InstallDirectURLRequest
	packageCalls    int
	packageMetadata *domain.PackageMetadata
}

func (installer *fakeRemoteInstaller) InstallDirectURL(ctx context.Context, req InstallDirectURLRequest) (*domain.App, error) {
	_ = ctx
	installer.directCalls++
	installer.directReq = req
	return &domain.App{ID: "direct", Name: "Direct App"}, nil
}

func (installer *fakeRemoteInstaller) InstallPackageMetadata(ctx context.Context, metadata *domain.PackageMetadata) (*domain.App, error) {
	_ = ctx
	installer.packageCalls++
	installer.packageMetadata = metadata
	return &domain.App{ID: "package", Name: "Package App"}, nil
}

type packageAmbiguityResolverFunc func(*domain.PackageMetadata) (*domain.PackageMetadata, error)

func (fn packageAmbiguityResolverFunc) ResolvePackageMetadataAmbiguity(view *domain.PackageMetadata) (*domain.PackageMetadata, error) {
	return fn(view)
}

type fakeDiscoveryBackend struct {
	metadata *domain.PackageMetadata
}

func (fakeDiscoveryBackend) Name() string {
	return "fake"
}

func (backend fakeDiscoveryBackend) Resolve(ctx context.Context, ref domain.PackageRef, assetOverride string) (*domain.PackageMetadata, error) {
	_ = ctx
	_ = ref
	_ = assetOverride
	return backend.metadata, nil
}
