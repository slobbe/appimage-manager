package services

import (
	"context"
	"fmt"
	"strings"
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
	source := &UpdateSourceInput{
		Kind: UpdateKindGitHubRelease,
		GitHubRelease: &GitHubReleaseUpdateView{
			Repo:  "owner/repo",
			Asset: "*.AppImage",
		},
	}
	service := SourceUpdateService{Store: store}

	plan, err := service.PlanSetSource(context.Background(), UpdateSourceRequest{ID: "app", Source: source})
	if err != nil {
		t.Fatalf("PlanSetSource returned error: %v", err)
	}
	if plan.Action != "set_update_source" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if plan.UpdateSourceChange == nil || plan.UpdateSourceChange.Incoming == nil || plan.UpdateSourceChange.Incoming.GitHubRelease == nil {
		t.Fatalf("plan missing typed update source change: %+v", plan.UpdateSourceChange)
	}

	result, err := service.SetSource(context.Background(), UpdateSourceRequest{ID: "app", Source: source})
	if err != nil {
		t.Fatalf("SetSource returned error: %v", err)
	}
	if !result.Changed || result.Source == nil || result.Source.GitHubRelease == nil || store.apps["app"].Update == nil || store.apps["app"].Update.GitHubRelease == nil {
		t.Fatalf("SetSource result = %+v app = %+v", result, store.apps["app"])
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

func TestDiscoveryWorkflowServiceUsesConfiguredBackends(t *testing.T) {
	metadata := &domain.PackageMetadata{Name: "App"}
	service := DiscoveryWorkflowService{Backends: []discovery.DiscoveryBackend{
		fakeDiscoveryBackend{metadata: metadata},
	}}

	got, err := service.ResolvePackage(context.Background(), PackageRefInfoRequest{
		Ref: ProviderRef{Provider: ProviderGitHub, Ref: "owner/repo"},
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
