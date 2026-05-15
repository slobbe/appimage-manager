package services

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	appimageapp "github.com/slobbe/appimage-manager/internal/app/appimage"
	"github.com/slobbe/appimage-manager/internal/app/discovery"
	appintegrate "github.com/slobbe/appimage-manager/internal/app/integrate"
	appremove "github.com/slobbe/appimage-manager/internal/app/remove"
	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	appupgrade "github.com/slobbe/appimage-manager/internal/app/upgrade"
	"github.com/slobbe/appimage-manager/internal/domain"
)

type AppStore interface {
	GetApp(id string) (*domain.App, error)
	GetAllApps() (map[string]*domain.App, error)
	UpdateApp(app *domain.App) error
}

type UpdateInfoReader interface {
	ReadUpdateInfo(path string) (*appupdate.UpdateInfo, error)
}

type UpdateInfoReaderFunc func(path string) (*appupdate.UpdateInfo, error)

func (fn UpdateInfoReaderFunc) ReadUpdateInfo(path string) (*appupdate.UpdateInfo, error) {
	return fn(path)
}

type AppImageInfoReader interface {
	ReadAppImageInfo(ctx context.Context, path string) (*appimageapp.AppInfo, error)
}

type AppImageInfoReaderFunc func(ctx context.Context, path string) (*appimageapp.AppInfo, error)

func (fn AppImageInfoReaderFunc) ReadAppImageInfo(ctx context.Context, path string) (*appimageapp.AppInfo, error) {
	return fn(ctx, path)
}

type IntegrateFunc func(context.Context, string, appintegrate.UpdateOverwritePrompt) (*domain.App, error)

type BasicAddService struct {
	IntegrateLocalApp IntegrateFunc
	ReintegrateApp    func(context.Context, string) (*domain.App, error)
	AppImageInfo      AppImageInfoReader
	AimDir            string
	DesktopDir        string
}

func NewBasicAddService(service BasicAddService) BasicAddService {
	return service
}

func (service BasicAddService) IntegrateLocal(ctx context.Context, req IntegrateLocalRequest) (*AddResult, error) {
	if service.IntegrateLocalApp == nil {
		return nil, fmt.Errorf("integrate local app service is not configured")
	}
	var prompt appintegrate.UpdateOverwritePrompt
	if req.ConfirmUpdateSourceReplace != nil {
		prompt = req.ConfirmUpdateSourceReplace.ConfirmUpdateSourceReplace
	}
	app, err := service.IntegrateLocalApp(ctx, req.Path, prompt)
	if err != nil {
		return nil, err
	}
	return &AddResult{Status: "integrated", App: app}, nil
}

func (service BasicAddService) Reintegrate(ctx context.Context, id string) (*AddResult, error) {
	if service.ReintegrateApp == nil {
		return nil, fmt.Errorf("reintegrate app service is not configured")
	}
	app, err := service.ReintegrateApp(ctx, id)
	if err != nil {
		return nil, err
	}
	return &AddResult{Status: "reintegrated", App: app}, nil
}

func (service BasicAddService) InstallDirectURL(ctx context.Context, req InstallDirectURLRequest) (*AddResult, error) {
	_ = ctx
	_ = req
	return nil, fmt.Errorf("direct URL install service is not configured")
}

func (service BasicAddService) InstallPackageRef(ctx context.Context, req InstallPackageRefRequest) (*AddResult, error) {
	_ = ctx
	_ = req
	return nil, fmt.Errorf("package install service is not configured")
}

func (service BasicAddService) PlanLocalIntegration(ctx context.Context, path string) (*DryRunPlan, error) {
	if service.AppImageInfo == nil {
		return nil, fmt.Errorf("appimage info reader is not configured")
	}
	info, err := service.AppImageInfo.ReadAppImageInfo(ctx, path)
	if err != nil {
		return nil, err
	}

	appID := strings.TrimSpace(info.ID)
	if appID == "" {
		appID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	appDir := filepath.Join(service.AimDir, appID)

	values := map[string]interface{}{
		"action": "integrate",
		"input":  strings.TrimSpace(path),
		"app_id": appID,
		"app": map[string]string{
			"name":    strings.TrimSpace(info.Name),
			"id":      appID,
			"version": strings.TrimSpace(info.Version),
		},
		"planned_paths": compactStrings([]string{
			filepath.Join(appDir, appID+".AppImage"),
			filepath.Join(appDir, appID+".desktop"),
			filepath.Join(service.DesktopDir, appID+".desktop"),
		}),
		"db_write": true,
	}
	return &DryRunPlan{Action: "integrate", Target: path, Values: values}, nil
}

func (service BasicAddService) PlanDirectURLInstall(ctx context.Context, req InstallDirectURLRequest) (*DryRunPlan, error) {
	_ = ctx
	values := map[string]interface{}{
		"action":          "install",
		"target":          strings.TrimSpace(req.URL),
		"target_kind":     "direct_url",
		"expected_sha256": strings.TrimSpace(req.SHA256),
		"download_url":    strings.TrimSpace(req.URL),
		"db_write":        true,
	}
	return &DryRunPlan{Action: "install", Target: req.URL, Values: values}, nil
}

func (service BasicAddService) PlanPackageRefInstall(ctx context.Context, req InstallPackageRefRequest) (*DryRunPlan, error) {
	_ = ctx
	_ = req
	return nil, fmt.Errorf("package install planning service is not configured")
}

type StoreListService struct {
	Store AppStore
}

func NewStoreListService(store AppStore) StoreListService {
	return StoreListService{Store: store}
}

func (service StoreListService) List(ctx context.Context, req ListRequest) (*ListResult, error) {
	_ = ctx
	if service.Store == nil {
		return nil, fmt.Errorf("app store is not configured")
	}
	apps, err := service.Store.GetAllApps()
	if err != nil {
		return nil, err
	}

	selected := make([]*domain.App, 0, len(apps))
	for _, app := range apps {
		if app == nil {
			continue
		}
		integrated := strings.TrimSpace(app.DesktopEntryLink) != ""
		if integrated && req.IncludeIntegrated {
			selected = append(selected, app)
		}
		if !integrated && req.IncludeUnlinked {
			selected = append(selected, app)
		}
	}
	return &ListResult{Apps: selected}, nil
}

type StoreInfoService struct {
	Store      AppStore
	AppImage   AppImageInfoReader
	UpdateInfo UpdateInfoReader
	Discovery  DiscoveryService
}

func NewStoreInfoService(service StoreInfoService) StoreInfoService {
	return service
}

func (service StoreInfoService) ManagedAppInfo(ctx context.Context, id string) (*InfoResult, error) {
	_ = ctx
	if service.Store == nil {
		return nil, fmt.Errorf("app store is not configured")
	}
	app, err := service.Store.GetApp(id)
	if err != nil {
		return nil, err
	}
	embedded, _ := service.embeddedUpdateSource(app.ExecPath)
	return &InfoResult{Kind: InfoKindManagedApp, App: app, EmbeddedUpdate: embedded}, nil
}

func (service StoreInfoService) LocalAppImageInfo(ctx context.Context, path string) (*InfoResult, error) {
	if service.AppImage == nil {
		return nil, fmt.Errorf("appimage info reader is not configured")
	}
	info, err := service.AppImage.ReadAppImageInfo(ctx, path)
	if err != nil {
		return nil, err
	}
	embedded, _ := service.embeddedUpdateSource(path)
	return &InfoResult{Kind: InfoKindLocalAppImage, AppImage: info, EmbeddedUpdate: embedded}, nil
}

func (service StoreInfoService) PackageRefInfo(ctx context.Context, req PackageRefInfoRequest) (*InfoResult, error) {
	if service.Discovery == nil {
		return nil, fmt.Errorf("discovery service is not configured")
	}
	metadata, err := service.Discovery.ResolvePackage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &InfoResult{Kind: InfoKindPackage, Package: metadata}, nil
}

func (service StoreInfoService) embeddedUpdateSource(path string) (*domain.UpdateSource, error) {
	if service.UpdateInfo == nil {
		return nil, fmt.Errorf("update info reader is not configured")
	}
	info, err := service.UpdateInfo.ReadUpdateInfo(strings.TrimSpace(path))
	if err != nil {
		return nil, err
	}
	if info == nil || info.Kind != domain.UpdateZsync {
		return nil, fmt.Errorf("unsupported embedded update info")
	}
	return &domain.UpdateSource{
		Kind: domain.UpdateZsync,
		Zsync: &domain.ZsyncUpdateSource{
			UpdateInfo: strings.TrimSpace(info.UpdateInfo),
			Transport:  strings.TrimSpace(info.Transport),
		},
	}, nil
}

type RemoveWorkflowService struct {
	Store      AppStore
	RemoveFunc func(context.Context, string, bool) (*domain.App, error)
}

func NewRemoveWorkflowService(service RemoveWorkflowService) RemoveWorkflowService {
	return service
}

func (service RemoveWorkflowService) Remove(ctx context.Context, req RemoveRequest) (*RemoveResult, error) {
	remove := service.RemoveFunc
	if remove == nil {
		remove = appremove.Remove
	}
	app, err := remove(ctx, req.ID, req.Unlink)
	if err != nil {
		return nil, err
	}
	return &RemoveResult{App: app, Unlink: req.Unlink, Paths: RemoveDryRunValues(app, req.Unlink)["paths"].([]string)}, nil
}

func (service RemoveWorkflowService) PlanRemove(ctx context.Context, req RemoveRequest) (*DryRunPlan, error) {
	_ = ctx
	if service.Store == nil {
		return nil, fmt.Errorf("app store is not configured")
	}
	app, err := service.Store.GetApp(req.ID)
	if err != nil {
		return nil, err
	}
	values := RemoveDryRunValues(app, req.Unlink)
	return &DryRunPlan{Action: values["action"].(string), Target: app.ID, Values: values}, nil
}

type DiscoveryWorkflowService struct {
	Backends     []discovery.DiscoveryBackend
	BackendsFunc func() []discovery.DiscoveryBackend
}

func NewDiscoveryWorkflowService(service DiscoveryWorkflowService) DiscoveryWorkflowService {
	return service
}

func (service DiscoveryWorkflowService) ResolvePackage(ctx context.Context, req PackageRefInfoRequest) (*domain.PackageMetadata, error) {
	backends := service.Backends
	if service.BackendsFunc != nil {
		backends = service.BackendsFunc()
	}
	for _, backend := range backends {
		if backend == nil {
			continue
		}
		metadata, err := backend.Resolve(ctx, req.Ref, req.AssetPattern)
		if err != nil {
			return nil, err
		}
		if metadata != nil {
			return metadata, nil
		}
	}
	return nil, fmt.Errorf("failed to resolve package metadata for %s", domain.FormatPackageRef(req.Ref))
}

type UpgradeWorkflowService struct {
	CheckFunc   func(context.Context, string) (*appupgrade.AimUpgradeCheckResult, error)
	UpgradeFunc func(context.Context, string) (*appupgrade.InstallerUpgradeResult, error)
}

func NewUpgradeWorkflowService(service UpgradeWorkflowService) UpgradeWorkflowService {
	return service
}

func (service UpgradeWorkflowService) Check(ctx context.Context, currentVersion string) (*appupgrade.AimUpgradeCheckResult, error) {
	check := service.CheckFunc
	if check == nil {
		check = appupgrade.CheckForAimUpgrade
	}
	return check(ctx, currentVersion)
}

func (service UpgradeWorkflowService) Upgrade(ctx context.Context, currentVersion string) (*appupgrade.InstallerUpgradeResult, error) {
	upgrade := service.UpgradeFunc
	if upgrade == nil {
		upgrade = appupgrade.UpgradeViaInstaller
	}
	return upgrade(ctx, currentVersion)
}

type SourceUpdateService struct {
	Store AppStore
}

func NewSourceUpdateService(store AppStore) SourceUpdateService {
	return SourceUpdateService{Store: store}
}

func (service SourceUpdateService) Check(ctx context.Context, req UpdateCheckRequest) (*UpdateCheckResult, error) {
	_ = ctx
	_ = req
	return nil, fmt.Errorf("update check service is not configured")
}

func (service SourceUpdateService) Apply(ctx context.Context, req UpdateApplyRequest) (*UpdateApplyResult, error) {
	_ = ctx
	_ = req
	return nil, fmt.Errorf("update apply service is not configured")
}

func (service SourceUpdateService) SetSource(ctx context.Context, req UpdateSourceRequest) (*UpdateSourceResult, error) {
	_ = ctx
	if service.Store == nil {
		return nil, fmt.Errorf("app store is not configured")
	}
	if err := domain.ValidateUpdateSource(req.Source); err != nil {
		return nil, err
	}
	app, err := service.Store.GetApp(req.ID)
	if err != nil {
		return nil, err
	}
	app.Update = req.Source
	if err := service.Store.UpdateApp(app); err != nil {
		return nil, err
	}
	return &UpdateSourceResult{ID: req.ID, Source: req.Source, Changed: true}, nil
}

func (service SourceUpdateService) UnsetSource(ctx context.Context, id string) (*UpdateSourceResult, error) {
	return service.SetSource(ctx, UpdateSourceRequest{ID: id, Source: &domain.UpdateSource{Kind: domain.UpdateNone}})
}

func (service SourceUpdateService) PlanSetSource(ctx context.Context, req UpdateSourceRequest) (*DryRunPlan, error) {
	_ = ctx
	if service.Store == nil {
		return nil, fmt.Errorf("app store is not configured")
	}
	app, err := service.Store.GetApp(req.ID)
	if err != nil {
		return nil, err
	}
	values := UpdateSetDryRunValues(req.ID, app.Update, req.Source)
	return &DryRunPlan{Action: "set_update_source", Target: req.ID, Values: values}, nil
}

func (service SourceUpdateService) PlanUnsetSource(ctx context.Context, id string) (*DryRunPlan, error) {
	_ = ctx
	if service.Store == nil {
		return nil, fmt.Errorf("app store is not configured")
	}
	app, err := service.Store.GetApp(id)
	if err != nil {
		return nil, err
	}
	values := UpdateUnsetDryRunValues(id, app.Update)
	return &DryRunPlan{Action: "unset_update_source", Target: id, Values: values}, nil
}

func RemoveDryRunValues(app *domain.App, unlink bool) map[string]interface{} {
	if app == nil {
		return map[string]interface{}{}
	}

	action := "remove"
	paths := []string{app.DesktopEntryLink}
	if unlink {
		action = "unlink"
	} else {
		paths = append(paths, filepath.Dir(app.ExecPath))
		if app.IconPath != "" {
			paths = append(paths, app.IconPath)
		}
	}

	return map[string]interface{}{
		"action": action,
		"unlink": unlink,
		"app":    app,
		"paths":  compactStrings(paths),
	}
}

func UpdateSetDryRunValues(id string, current, incoming *domain.UpdateSource) map[string]interface{} {
	return map[string]interface{}{
		"action":          "set_update_source",
		"id":              strings.TrimSpace(id),
		"current_source":  current,
		"incoming_source": incoming,
		"db_write":        true,
	}
}

func UpdateUnsetDryRunValues(id string, current *domain.UpdateSource) map[string]interface{} {
	return map[string]interface{}{
		"action":         "unset_update_source",
		"id":             strings.TrimSpace(id),
		"current_source": current,
		"db_write":       true,
	}
}

func compactStrings(values []string) []string {
	filtered := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		filtered = append(filtered, trimmed)
	}
	return filtered
}
