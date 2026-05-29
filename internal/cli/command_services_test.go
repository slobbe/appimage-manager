package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	appservices "github.com/slobbe/appimage-manager/internal/app/services"
	"github.com/spf13/cobra"
)

func TestListCmdCallsListServiceWithParsedFilter(t *testing.T) {
	service := &recordingListService{
		result: &appservices.ListResult{Apps: []*appservices.AppDetails{{
			AppSummary: appservices.AppSummary{ID: "app", Name: "App", Integrated: true},
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
		result: &appservices.RemoveResult{Action: "remove", App: &appservices.AppDetails{AppSummary: appservices.AppSummary{ID: "app", Name: "App"}}},
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
	service := &recordingUpgradeService{}
	cmd := newUpgradeTestCommand()
	ctx := withRuntimeServices(context.Background(), runtimeServices{Upgrade: service, Locker: noopLocker{}})

	if err := executeTestCommand(ctx, cmd, "--upgrade"); err != nil {
		t.Fatalf("upgrade command returned error: %v", err)
	}

	if service.checkCalls != 1 || service.upgradeCalls != 1 {
		t.Fatalf("upgrade service calls = check %d upgrade %d, want 1/1", service.checkCalls, service.upgradeCalls)
	}
}

func TestAddCmdDryRunRoutesLocalAppImageToPlan(t *testing.T) {
	service := &recordingAddService{
		resolveTarget: &appservices.IntegrateTargetResult{Kind: appservices.IntegrateTargetLocalFile, LocalPath: "/tmp/My.AppImage"},
		plan: &appservices.DryRunPlan{Values: map[string]interface{}{
			"input":         "/tmp/My.AppImage",
			"app_id":        "my-app",
			"planned_paths": []string{"/tmp/aim/my-app.AppImage", "/tmp/aim/my-app.desktop"},
		}},
	}
	cmd, output := commandWithRuntimeForTest(newAddCommand(), runtimeOptions{DryRun: true}, runtimeServices{Add: service, Locker: noopLocker{}})

	if err := AddCmd(cmd, []string{"/tmp/My.AppImage"}); err != nil {
		t.Fatalf("AddCmd returned error: %v", err)
	}

	if service.resolveCalls != 2 {
		t.Fatalf("ResolveIntegrateTarget calls = %d, want 2 for current command trace", service.resolveCalls)
	}
	if service.planLocalPath != "/tmp/My.AppImage" {
		t.Fatalf("PlanLocalIntegration path = %q", service.planLocalPath)
	}
	if service.integrateLocalCalls != 0 {
		t.Fatalf("IntegrateLocal calls = %d, want 0", service.integrateLocalCalls)
	}
	if got := output.String(); !strings.Contains(got, "Dry run: would integrate /tmp/My.AppImage") || !strings.Contains(got, "Managed ID: my-app") {
		t.Fatalf("unexpected output:\n%s", got)
	}
}

func TestAddCmdDryRunRoutesDirectURLToPlan(t *testing.T) {
	service := &recordingAddService{plan: &appservices.DryRunPlan{Values: map[string]interface{}{"target": "https://example.com/My.AppImage"}}}
	cmd, output := commandWithRuntimeForTest(newAddCommand(), runtimeOptions{DryRun: true}, runtimeServices{Add: service, Locker: noopLocker{}})
	if err := cmd.Flags().Set("url", "https://example.com/My.AppImage"); err != nil {
		t.Fatalf("set url: %v", err)
	}
	if err := cmd.Flags().Set("sha256", strings.Repeat("a", 64)); err != nil {
		t.Fatalf("set sha256: %v", err)
	}

	if err := AddCmd(cmd, nil); err != nil {
		t.Fatalf("AddCmd returned error: %v", err)
	}

	if service.planDirectReq.URL != "https://example.com/My.AppImage" || service.planDirectReq.SHA256 != strings.Repeat("a", 64) {
		t.Fatalf("PlanDirectURLInstall request = %+v", service.planDirectReq)
	}
	if service.installDirectCalls != 0 {
		t.Fatalf("InstallDirectURL calls = %d, want 0", service.installDirectCalls)
	}
	if got := output.String(); !strings.Contains(got, "Dry run: would install https://example.com/My.AppImage") {
		t.Fatalf("unexpected output:\n%s", got)
	}
}

func TestAddCmdDryRunRoutesPackageRefToPlan(t *testing.T) {
	service := &recordingAddService{plan: &appservices.DryRunPlan{Values: map[string]interface{}{"target": "github:owner/repo"}}}
	cmd, output := commandWithRuntimeForTest(newAddCommand(), runtimeOptions{DryRun: true}, runtimeServices{Add: service, Locker: noopLocker{}})
	if err := cmd.Flags().Set("github", "owner/repo"); err != nil {
		t.Fatalf("set github: %v", err)
	}
	if err := cmd.Flags().Set("asset", "*.AppImage"); err != nil {
		t.Fatalf("set asset: %v", err)
	}

	if err := AddCmd(cmd, nil); err != nil {
		t.Fatalf("AddCmd returned error: %v", err)
	}

	if service.planPackageReq.Ref.Provider != appservices.ProviderGitHub || service.planPackageReq.Ref.Ref != "owner/repo" || service.planPackageReq.AssetPattern != "*.AppImage" {
		t.Fatalf("PlanPackageRefInstall request = %+v", service.planPackageReq)
	}
	if service.installPackageCalls != 0 {
		t.Fatalf("InstallPackageRef calls = %d, want 0", service.installPackageCalls)
	}
	if got := output.String(); !strings.Contains(got, "Dry run: would install GitHub owner/repo") {
		t.Fatalf("unexpected output:\n%s", got)
	}
}

func TestAddCmdReintegrateAndAlreadyIntegratedDoNotTakeWriteLockInDryRun(t *testing.T) {
	tests := []struct {
		name       string
		target     *appservices.IntegrateTargetResult
		wantOutput string
	}{
		{
			name:       "already integrated",
			target:     &appservices.IntegrateTargetResult{Kind: appservices.IntegrateTargetIntegrated, App: appDetails("app", "App", true)},
			wantOutput: "Already integrated: App",
		},
		{
			name:       "reintegrate unlinked dry-run",
			target:     &appservices.IntegrateTargetResult{Kind: appservices.IntegrateTargetUnlinked, App: appDetails("app", "App", false)},
			wantOutput: "Dry run: would reintegrate App [app]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &recordingAddService{resolveTarget: tt.target}
			locker := &recordingLocker{}
			cmd, output := commandWithRuntimeForTest(newAddCommand(), runtimeOptions{DryRun: true}, runtimeServices{Add: service, Locker: locker})

			if err := AddCmd(cmd, []string{"app"}); err != nil {
				t.Fatalf("AddCmd returned error: %v", err)
			}

			if locker.called {
				t.Fatal("dry-run add should not use write lock")
			}
			if service.reintegrateCalls != 0 || service.integrateLocalCalls != 0 {
				t.Fatalf("unexpected mutating calls: reintegrate=%d integrate=%d", service.reintegrateCalls, service.integrateLocalCalls)
			}
			if got := output.String(); !strings.Contains(got, tt.wantOutput) {
				t.Fatalf("unexpected output:\n%s", got)
			}
		})
	}
}

func TestRemoveCmdDryRunUsesRemoveServiceWithoutWriteLock(t *testing.T) {
	remove := &recordingRemoveService{
		result: &appservices.RemoveResult{Action: "remove", App: appDetails("app", "App", true), Paths: []string{"/tmp/app.AppImage"}},
	}
	locker := &recordingLocker{}
	cmd, output := commandWithRuntimeForTest(newRemoveCommand(), runtimeOptions{DryRun: true}, runtimeServices{Remove: remove, Locker: locker})

	if err := RemoveCmd(cmd, []string{"app"}); err != nil {
		t.Fatalf("RemoveCmd returned error: %v", err)
	}

	if remove.calls != 1 {
		t.Fatalf("Remove calls = %d, want 1", remove.calls)
	}
	if !remove.req.DryRun {
		t.Fatalf("Remove request = %+v, want dry-run", remove.req)
	}
	if locker.called {
		t.Fatal("dry-run remove should not use write lock")
	}
	if got := output.String(); !strings.Contains(got, "Dry run: would remove App [app]") || !strings.Contains(got, "/tmp/app.AppImage") {
		t.Fatalf("unexpected output:\n%s", got)
	}
}

func TestRemoveCmdUnlinkUsesRemoveServiceAndLocker(t *testing.T) {
	remove := &recordingRemoveService{result: &appservices.RemoveResult{Action: "unlink", App: appDetails("app", "App", false), Unlink: true}}
	locker := &recordingLocker{}
	cmd, output := commandWithRuntimeForTest(newRemoveCommand(), runtimeOptions{}, runtimeServices{Remove: remove, Locker: locker})
	if err := cmd.Flags().Set("link", "true"); err != nil {
		t.Fatalf("set link: %v", err)
	}

	if err := RemoveCmd(cmd, []string{"app"}); err != nil {
		t.Fatalf("RemoveCmd returned error: %v", err)
	}

	if !locker.called {
		t.Fatal("expected unlink remove to use write lock")
	}
	if remove.req.ID != "app" || !remove.req.Unlink {
		t.Fatalf("Remove request = %+v, want id app unlink true", remove.req)
	}
	if got := output.String(); !strings.Contains(got, "Unlinked: App [app]") {
		t.Fatalf("unexpected output:\n%s", got)
	}
}

func TestListCmdFiltersAndOutputFormats(t *testing.T) {
	apps := []*appservices.AppDetails{
		appDetails("alpha", "Alpha", true),
		appDetails("beta", "Beta", false),
	}
	tests := []struct {
		name       string
		flags      map[string]string
		opts       runtimeOptions
		want       []string
		wantAbsent []string
	}{
		{name: "all", flags: map[string]string{"all": "true"}, want: []string{"alpha", "beta"}},
		{name: "integrated", flags: map[string]string{"integrated": "true"}, want: []string{"alpha"}, wantAbsent: []string{"beta"}},
		{name: "unlinked", flags: map[string]string{"unlinked": "true"}, want: []string{"beta"}, wantAbsent: []string{"alpha"}},
		{name: "json", opts: runtimeOptions{JSON: true}, want: []string{`"id": "alpha"`, `"id": "beta"`}},
		{name: "csv", opts: runtimeOptions{CSV: true}, want: []string{"id,name,version,integrated,exec_path,update_available,latest_version,last_checked_at", "alpha", "beta"}},
		{name: "plain", opts: runtimeOptions{Plain: true}, want: []string{"alpha", "beta"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &recordingListService{result: &appservices.ListResult{Apps: apps}}
			cmd, output := commandWithRuntimeForTest(newListCommand(), tt.opts, runtimeServices{List: service, Locker: noopLocker{}})
			for name, value := range tt.flags {
				if err := cmd.Flags().Set(name, value); err != nil {
					t.Fatalf("set %s: %v", name, err)
				}
			}

			if err := ListCmd(cmd, nil); err != nil {
				t.Fatalf("ListCmd returned error: %v", err)
			}

			got := output.String()
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("output missing %q:\n%s", want, got)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Fatalf("output contains %q:\n%s", absent, got)
				}
			}
		})
	}
}

func TestInfoCmdRoutesManagedLocalAndPackageTargets(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		flags      map[string]string
		wantCall   string
		wantOutput string
	}{
		{name: "managed app", args: []string{"app"}, wantCall: "managed", wantOutput: "Name: App"},
		{name: "local AppImage", args: []string{"/tmp/App.AppImage"}, wantCall: "local", wantOutput: "Path: /tmp/App.AppImage"},
		{name: "package ref", flags: map[string]string{"github": "owner/repo"}, wantCall: "package", wantOutput: "GitHub"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &recordingInfoService{
				managed: &appservices.InfoResult{AppDetails: appDetails("app", "App", true)},
				local:   &appservices.InfoResult{AppImageInfo: &appservices.AppImageInfoView{Name: "App", ID: "app", Version: "1.0.0"}},
				pkg:     &appservices.InfoResult{PackageView: &appservices.PackageView{Name: "App", Provider: "GitHub", Ref: appservices.ProviderRef{Provider: appservices.ProviderGitHub, Ref: "owner/repo"}, LatestVersion: "1.0.0", Installable: true}},
			}
			cmd, output := commandWithRuntimeForTest(newInfoCommand(), runtimeOptions{}, runtimeServices{Info: service, Locker: noopLocker{}})
			for name, value := range tt.flags {
				if err := cmd.Flags().Set(name, value); err != nil {
					t.Fatalf("set %s: %v", name, err)
				}
			}

			if err := InfoCmd(cmd, tt.args); err != nil {
				t.Fatalf("InfoCmd returned error: %v", err)
			}

			if service.lastCall != tt.wantCall {
				t.Fatalf("last info call = %q, want %q", service.lastCall, tt.wantCall)
			}
			if got := output.String(); !strings.Contains(got, tt.wantOutput) {
				t.Fatalf("unexpected output:\n%s", got)
			}
		})
	}
}

func commandWithRuntimeForTest(cmd *cobra.Command, opts runtimeOptions, services runtimeServices) (*cobra.Command, *bytes.Buffer) {
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	if cmd.Flags().Lookup("yes") == nil && cmd.InheritedFlags().Lookup("yes") == nil {
		cmd.Flags().BoolP("yes", "y", false, "skip confirmation prompts")
	}
	ctx := context.WithValue(context.Background(), runtimeContextKey{}, opts)
	cmd.SetContext(withRuntimeServices(ctx, services))
	return cmd, &output
}

func appDetails(id, name string, integrated bool) *appservices.AppDetails {
	return &appservices.AppDetails{AppSummary: appservices.AppSummary{ID: id, Name: name, Version: "1.0.0", Integrated: integrated}}
}

type recordingAddService struct {
	resolveCalls        int
	resolveTarget       *appservices.IntegrateTargetResult
	integrateLocalCalls int
	reintegrateCalls    int
	installDirectCalls  int
	installPackageCalls int
	planLocalPath       string
	planDirectReq       appservices.InstallDirectURLRequest
	planPackageReq      appservices.InstallPackageRefRequest
	plan                *appservices.DryRunPlan
}

func (service *recordingAddService) ResolveIntegrateTarget(ctx context.Context, input string) (*appservices.IntegrateTargetResult, error) {
	_ = ctx
	_ = input
	service.resolveCalls++
	if service.resolveTarget == nil {
		return nil, errors.New("not found")
	}
	return service.resolveTarget, nil
}

func (service *recordingAddService) IntegrateLocal(ctx context.Context, req appservices.IntegrateLocalRequest) (*appservices.AddResult, error) {
	_ = ctx
	_ = req
	service.integrateLocalCalls++
	return &appservices.AddResult{App: appDetails("app", "App", true)}, nil
}

func (service *recordingAddService) Reintegrate(ctx context.Context, id string) (*appservices.AddResult, error) {
	_ = ctx
	_ = id
	service.reintegrateCalls++
	return &appservices.AddResult{App: appDetails("app", "App", true)}, nil
}

func (service *recordingAddService) InstallDirectURL(ctx context.Context, req appservices.InstallDirectURLRequest) (*appservices.AddResult, error) {
	_ = ctx
	_ = req
	service.installDirectCalls++
	return &appservices.AddResult{App: appDetails("app", "App", true)}, nil
}

func (service *recordingAddService) InstallPackageRef(ctx context.Context, req appservices.InstallPackageRefRequest) (*appservices.AddResult, error) {
	_ = ctx
	_ = req
	service.installPackageCalls++
	return &appservices.AddResult{App: appDetails("app", "App", true)}, nil
}

func (service *recordingAddService) PlanLocalIntegration(ctx context.Context, path string) (*appservices.DryRunPlan, error) {
	_ = ctx
	service.planLocalPath = path
	return service.plan, nil
}

func (service *recordingAddService) PlanDirectURLInstall(ctx context.Context, req appservices.InstallDirectURLRequest) (*appservices.DryRunPlan, error) {
	_ = ctx
	service.planDirectReq = req
	return service.plan, nil
}

func (service *recordingAddService) PlanPackageRefInstall(ctx context.Context, req appservices.InstallPackageRefRequest) (*appservices.DryRunPlan, error) {
	_ = ctx
	service.planPackageReq = req
	return service.plan, nil
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
	calls  int
	req    appservices.RemoveRequest
	result *appservices.RemoveResult
}

func (service *recordingRemoveService) Remove(ctx context.Context, req appservices.RemoveRequest) (*appservices.RemoveResult, error) {
	_ = ctx
	service.calls++
	service.req = req
	return service.result, nil
}

type recordingInfoService struct {
	lastCall string
	managed  *appservices.InfoResult
	local    *appservices.InfoResult
	pkg      *appservices.InfoResult
}

func (service *recordingInfoService) ManagedAppInfo(ctx context.Context, id string) (*appservices.InfoResult, error) {
	_ = ctx
	_ = id
	service.lastCall = "managed"
	return service.managed, nil
}

func (service *recordingInfoService) LocalAppImageInfo(ctx context.Context, path string) (*appservices.InfoResult, error) {
	_ = ctx
	_ = path
	service.lastCall = "local"
	return service.local, nil
}

func (service *recordingInfoService) PackageRefInfo(ctx context.Context, req appservices.PackageRefInfoRequest) (*appservices.InfoResult, error) {
	_ = ctx
	_ = req
	service.lastCall = "package"
	return service.pkg, nil
}

type recordingUpgradeService struct {
	checkCalls   int
	upgradeCalls int
}

func (service *recordingUpgradeService) Check(ctx context.Context, currentVersion string) (*appservices.AimUpgradeCheckResult, error) {
	_ = ctx
	_ = currentVersion
	service.checkCalls++
	return nil, nil
}

func (service *recordingUpgradeService) Upgrade(ctx context.Context, currentVersion string) (*appservices.InstallerUpgradeResult, error) {
	_ = ctx
	_ = currentVersion
	service.upgradeCalls++
	return nil, nil
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
