package services

import (
	"context"
	"testing"

	"github.com/slobbe/appimage-manager/internal/app/discovery"
	"github.com/slobbe/appimage-manager/internal/domain"
)

func TestStoreListServiceFiltersApps(t *testing.T) {
	store := fakeAppStore{apps: map[string]*domain.App{
		"integrated": {ID: "integrated", DesktopEntryLink: "/tmp/integrated.desktop"},
		"unlinked":   {ID: "unlinked"},
	}}

	result, err := (StoreListService{Store: store}).List(context.Background(), ListRequest{IncludeIntegrated: true})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(result.Apps) != 1 || result.Apps[0].ID != "integrated" || !result.Apps[0].Integrated {
		t.Fatalf("List returned %+v, want integrated app only", result.Apps)
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

	plan, err := service.PlanSetSource(context.Background(), UpdateSourceRequest{ID: "app", Source: source})
	if err != nil {
		t.Fatalf("PlanSetSource returned error: %v", err)
	}
	if plan.Action != "set_update_source" || plan.Values["incoming_source"] != source {
		t.Fatalf("unexpected plan: %+v", plan)
	}

	result, err := service.SetSource(context.Background(), UpdateSourceRequest{ID: "app", Source: source})
	if err != nil {
		t.Fatalf("SetSource returned error: %v", err)
	}
	if !result.Changed || store.apps["app"].Update != source {
		t.Fatalf("SetSource result = %+v app = %+v", result, store.apps["app"])
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
