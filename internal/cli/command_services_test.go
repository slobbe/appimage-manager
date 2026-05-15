package cli

import (
	"context"
	"testing"

	appservices "github.com/slobbe/appimage-manager/internal/app/services"
	appupgrade "github.com/slobbe/appimage-manager/internal/app/upgrade"
	"github.com/slobbe/appimage-manager/internal/domain"
)

func TestListCmdCallsListServiceWithParsedFilter(t *testing.T) {
	service := &recordingListService{
		result: &appservices.ListResult{Apps: []*domain.App{{
			ID:               "app",
			Name:             "App",
			DesktopEntryLink: "/tmp/app.desktop",
		}}},
	}
	cmd := newListCommand()
	installRuntimeServicesForTest(cmd, runtimeServices{List: service, Locker: noopLocker{}})

	if err := cmd.Flags().Set("integrated", "true"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	if err := ListCmd(cmd, nil); err != nil {
		t.Fatalf("ListCmd returned error: %v", err)
	}

	if service.calls != 1 {
		t.Fatalf("List calls = %d, want 1", service.calls)
	}
	if !service.req.IncludeIntegrated || !service.req.IncludeUnlinked {
		t.Fatalf("List request = %+v, want command to load both states for rendering", service.req)
	}
}

func TestRemoveCmdUsesRemoveServiceAndLocker(t *testing.T) {
	remove := &recordingRemoveService{
		result: &appservices.RemoveResult{App: &domain.App{ID: "app", Name: "App"}},
	}
	locker := &recordingLocker{}
	cmd := newRemoveCommand()
	installRuntimeServicesForTest(cmd, runtimeServices{Remove: remove, Locker: locker})

	if err := RemoveCmd(cmd, []string{"app"}); err != nil {
		t.Fatalf("RemoveCmd returned error: %v", err)
	}

	if !locker.called {
		t.Fatal("expected remove command to use runtime locker")
	}
	if remove.calls != 1 {
		t.Fatalf("Remove calls = %d, want 1", remove.calls)
	}
	if remove.req.ID != "app" || remove.req.Unlink {
		t.Fatalf("Remove request = %+v, want id app unlink false", remove.req)
	}
}

func TestUpgradeCmdCallsUpgradeService(t *testing.T) {
	service := &recordingUpgradeService{
		check: &appupgrade.AimUpgradeCheckResult{
			CurrentVersion: "0.1.0",
			LatestVersion:  "0.2.0",
			HasUpdate:      true,
			Comparable:     true,
		},
		upgrade: &appupgrade.InstallerUpgradeResult{
			PreviousVersion:  "0.1.0",
			InstalledVersion: "0.2.0",
		},
	}
	cmd := newUpgradeTestCommand()
	ctx := withRuntimeServices(context.Background(), runtimeServices{Upgrade: service, Locker: noopLocker{}})

	if err := executeTestCommand(ctx, cmd, "--upgrade"); err != nil {
		t.Fatalf("upgrade command returned error: %v", err)
	}

	if service.checkCalls != 1 || service.upgradeCalls != 1 {
		t.Fatalf("upgrade service calls = check %d upgrade %d, want 1/1", service.checkCalls, service.upgradeCalls)
	}
}

type recordingListService struct {
	calls  int
	req    appservices.ListRequest
	result *appservices.ListResult
}

func (service *recordingListService) List(ctx context.Context, req appservices.ListRequest) (*appservices.ListResult, error) {
	_ = ctx
	service.calls++
	service.req = req
	return service.result, nil
}

type recordingRemoveService struct {
	calls     int
	planCalls int
	req       appservices.RemoveRequest
	result    *appservices.RemoveResult
}

func (service *recordingRemoveService) Remove(ctx context.Context, req appservices.RemoveRequest) (*appservices.RemoveResult, error) {
	_ = ctx
	service.calls++
	service.req = req
	return service.result, nil
}

func (service *recordingRemoveService) PlanRemove(ctx context.Context, req appservices.RemoveRequest) (*appservices.DryRunPlan, error) {
	_ = ctx
	service.planCalls++
	service.req = req
	return &appservices.DryRunPlan{Values: removeDryRunPlan(service.result.App, req.Unlink)}, nil
}

type recordingUpgradeService struct {
	checkCalls   int
	upgradeCalls int
	check        *appupgrade.AimUpgradeCheckResult
	upgrade      *appupgrade.InstallerUpgradeResult
}

func (service *recordingUpgradeService) Check(ctx context.Context, currentVersion string) (*appupgrade.AimUpgradeCheckResult, error) {
	_ = ctx
	_ = currentVersion
	service.checkCalls++
	return service.check, nil
}

func (service *recordingUpgradeService) Upgrade(ctx context.Context, currentVersion string) (*appupgrade.InstallerUpgradeResult, error) {
	_ = ctx
	_ = currentVersion
	service.upgradeCalls++
	return service.upgrade, nil
}

type recordingLocker struct {
	called bool
}

func (locker *recordingLocker) WithWriteLock(fn func() error) error {
	locker.called = true
	return fn()
}

type noopLocker struct{}

func (noopLocker) WithWriteLock(fn func() error) error {
	return fn()
}
