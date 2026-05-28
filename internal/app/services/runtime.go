package services

import (
	"context"
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
	Store                     AppStore
	HasExtension              func(string, string) bool
	IntegrateLocalApp         IntegrateFunc
	ReintegrateApp            func(context.Context, string) (*domain.App, error)
	InstallDirectURLApp       func(context.Context, InstallDirectURLRequest) (*domain.App, error)
	InstallPackageRefApp      func(context.Context, *domain.PackageMetadata) (*domain.App, error)
	PlanPackageRefInstallFunc func(context.Context, InstallPackageRefRequest) (*DryRunPlan, error)
	Discovery                 DiscoveryService
	AppImageInfo              AppImageInfoReader
	AimDir                    string
	DesktopDir                string
}

func NewBasicAddService(service BasicAddService) BasicAddService {
	return service
}

func (service BasicAddService) ResolveIntegrateTarget(ctx context.Context, input string) (*IntegrateTargetResult, error) {
	_ = ctx
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, invalidInputErrorf("missing required argument <Path/To.AppImage|id>")
	}

	if service.Store != nil {
		if app, err := service.Store.GetApp(trimmed); err == nil && app != nil {
			kind := IntegrateTargetIntegrated
			if strings.TrimSpace(app.DesktopEntryLink) == "" {
				kind = IntegrateTargetUnlinked
			}
			return &IntegrateTargetResult{Kind: kind, App: app}, nil
		}
	}

	if strings.HasPrefix(trimmed, "https://") {
		return nil, invalidInputErrorf("remote sources are added with 'aim add'")
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "http://") {
		return nil, invalidInputErrorf("direct URLs must use https; use 'aim add --url https://...'")
	}

	hasExtension := service.HasExtension
	if hasExtension == nil {
		hasExtension = func(value, ext string) bool { return strings.EqualFold(filepath.Ext(value), ext) }
	}
	if hasExtension(trimmed, ".AppImage") {
		return &IntegrateTargetResult{Kind: IntegrateTargetLocalFile, LocalPath: trimmed}, nil
	}

	return nil, invalidInputErrorf("unknown argument %s", input)
}

func (service BasicAddService) IntegrateLocal(ctx context.Context, req IntegrateLocalRequest) (*AddResult, error) {
	if service.IntegrateLocalApp == nil {
		return nil, internalErrorf("integrate local app service is not configured")
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
		return nil, internalErrorf("reintegrate app service is not configured")
	}
	app, err := service.ReintegrateApp(ctx, id)
	if err != nil {
		return nil, err
	}
	return &AddResult{Status: "reintegrated", App: app}, nil
}

func (service BasicAddService) InstallDirectURL(ctx context.Context, req InstallDirectURLRequest) (*AddResult, error) {
	if service.InstallDirectURLApp == nil {
		return nil, internalErrorf("direct URL install service is not configured")
	}
	app, err := service.InstallDirectURLApp(ctx, req)
	if err != nil {
		return nil, err
	}
	return &AddResult{Status: "installed", App: app}, nil
}

func (service BasicAddService) InstallPackageRef(ctx context.Context, req InstallPackageRefRequest) (*AddResult, error) {
	if service.Discovery == nil && req.ResolveMetadata == nil {
		return nil, internalErrorf("discovery service is not configured")
	}
	install := service.InstallPackageRefApp
	if req.InstallPackage != nil {
		install = req.InstallPackage
	}
	if install == nil {
		return nil, internalErrorf("package install service is not configured")
	}
	var metadata *domain.PackageMetadata
	var err error
	if req.ResolveMetadata != nil {
		metadata, err = req.ResolveMetadata(ctx, req.Ref, req.AssetPattern)
	} else {
		metadata, err = service.Discovery.ResolvePackage(ctx, PackageRefInfoRequest{Ref: req.Ref, AssetPattern: req.AssetPattern})
	}
	if err != nil {
		return nil, err
	}
	metadata, err = RequireInstallablePackage(metadata)
	if err != nil {
		return nil, err
	}
	if req.ResolveAmbiguity != nil {
		metadata, err = req.ResolveAmbiguity.ResolvePackageAmbiguity(metadata)
		if err != nil {
			return nil, err
		}
	}
	app, err := install(ctx, metadata)
	if err != nil {
		return nil, err
	}
	return &AddResult{Status: "installed", App: app}, nil
}

func (service BasicAddService) PlanLocalIntegration(ctx context.Context, path string) (*DryRunPlan, error) {
	if service.AppImageInfo == nil {
		return nil, internalErrorf("appimage info reader is not configured")
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
	if service.PlanPackageRefInstallFunc != nil {
		return service.PlanPackageRefInstallFunc(ctx, req)
	}
	if service.Discovery == nil {
		return nil, internalErrorf("discovery service is not configured")
	}
	metadata, err := service.Discovery.ResolvePackage(ctx, PackageRefInfoRequest{Ref: req.Ref, AssetPattern: req.AssetPattern})
	if err != nil {
		return nil, err
	}
	values := map[string]interface{}{
		"action":   "install",
		"target":   domain.FormatPackageRef(req.Ref),
		"provider": req.Ref,
		"metadata": metadata,
	}
	return &DryRunPlan{Action: "install", Target: domain.FormatPackageRef(req.Ref), Values: values}, nil
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
		return nil, internalErrorf("app store is not configured")
	}
	apps, err := service.Store.GetAllApps()
	if err != nil {
		return nil, err
	}

	selected := make([]*AppDetails, 0, len(apps))
	managedApps := make([]*domain.App, 0, len(apps))
	for _, app := range apps {
		if app == nil {
			continue
		}
		integrated := strings.TrimSpace(app.DesktopEntryLink) != ""
		if integrated && req.IncludeIntegrated {
			selected = append(selected, appDetailsFromDomain(app))
			managedApps = append(managedApps, app)
		}
		if !integrated && req.IncludeUnlinked {
			selected = append(selected, appDetailsFromDomain(app))
			managedApps = append(managedApps, app)
		}
	}
	return &ListResult{Apps: selected, ManagedApps: managedApps}, nil
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
		return nil, internalErrorf("app store is not configured")
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
		return nil, internalErrorf("appimage info reader is not configured")
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
		return nil, internalErrorf("discovery service is not configured")
	}
	metadata, err := service.Discovery.ResolvePackage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &InfoResult{Kind: InfoKindPackage, Package: metadata}, nil
}

func (service StoreInfoService) embeddedUpdateSource(path string) (*domain.UpdateSource, error) {
	if service.UpdateInfo == nil {
		return nil, internalErrorf("update info reader is not configured")
	}
	info, err := service.UpdateInfo.ReadUpdateInfo(strings.TrimSpace(path))
	if err != nil {
		return nil, err
	}
	if info == nil || info.Kind != domain.UpdateZsync {
		return nil, internalErrorf("unsupported embedded update info")
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
	return removeResultFromDomain(app, req.Unlink), nil
}

func (service RemoveWorkflowService) PlanRemove(ctx context.Context, req RemoveRequest) (*DryRunPlan, error) {
	_ = ctx
	if service.Store == nil {
		return nil, internalErrorf("app store is not configured")
	}
	app, err := service.Store.GetApp(req.ID)
	if err != nil {
		return nil, err
	}
	values := RemoveDryRunValues(app, req.Unlink)
	result := removeResultFromDomain(app, req.Unlink)
	return &DryRunPlan{
		Action:     result.Action,
		Target:     app.ID,
		Values:     values,
		TargetKind: "managed_app",
		App:        result.App,
		Paths:      result.Paths,
		DBWrite:    !req.Unlink,
	}, nil
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
	return discovery.NewService(backends...).Resolve(ctx, req.Ref, req.AssetPattern)
}

type UpgradeWorkflowService struct {
	CheckFunc   func(context.Context, string) (*appupgrade.AimUpgradeCheckResult, error)
	UpgradeFunc func(context.Context, string) (*appupgrade.InstallerUpgradeResult, error)
}

func NewUpgradeWorkflowService(service UpgradeWorkflowService) UpgradeWorkflowService {
	return service
}

func (service UpgradeWorkflowService) Check(ctx context.Context, currentVersion string) (*appupgrade.AimUpgradeCheckResult, error) {
	if service.CheckFunc == nil {
		return nil, internalErrorf("upgrade check service is not configured")
	}
	return service.CheckFunc(ctx, currentVersion)
}

func (service UpgradeWorkflowService) Upgrade(ctx context.Context, currentVersion string) (*appupgrade.InstallerUpgradeResult, error) {
	if service.UpgradeFunc == nil {
		return nil, internalErrorf("upgrade installer service is not configured")
	}
	return service.UpgradeFunc(ctx, currentVersion)
}

type ManagedUpdateChecker func([]*domain.App, appupdate.ManagedCheckFunc) []appupdate.ManagedCheckResult

type ManagedUpdateApplier func(context.Context, appupdate.ManagedUpdate, appupdate.ManagedApplyReporter) (*domain.App, error)

type SourceUpdateService struct {
	Store               AppStore
	CheckManagedUpdates ManagedUpdateChecker
	ApplyManagedUpdate  ManagedUpdateApplier
}

func NewSourceUpdateService(store AppStore) SourceUpdateService {
	return SourceUpdateService{Store: store}
}

func NewSourceUpdateWorkflowService(service SourceUpdateService) SourceUpdateService {
	return service
}

func (service SourceUpdateService) Check(ctx context.Context, req UpdateCheckRequest) (*UpdateCheckResult, error) {
	_ = ctx
	if service.Store == nil {
		return nil, internalErrorf("app store is not configured")
	}

	apps := make([]*domain.App, 0)
	if strings.TrimSpace(req.ID) != "" {
		app, err := service.Store.GetApp(req.ID)
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	} else {
		all, err := service.Store.GetAllApps()
		if err != nil {
			return nil, err
		}
		for _, app := range all {
			if app != nil {
				apps = append(apps, app)
			}
		}
	}

	check := service.CheckManagedUpdates
	if check == nil {
		check = appupdate.CheckManagedUpdates
	}
	checkResults := check(apps, nil)
	statuses := make([]ManagedUpdateStatus, 0, len(checkResults))
	for _, result := range checkResults {
		statuses = append(statuses, ManagedUpdateStatus{App: result.App, Update: result.Update, Error: result.Error})
	}
	return &UpdateCheckResult{Apps: statuses}, nil
}

func (service SourceUpdateService) Apply(ctx context.Context, req UpdateApplyRequest) (*UpdateApplyResult, error) {
	if service.Store == nil {
		return nil, internalErrorf("app store is not configured")
	}
	apply := service.ApplyManagedUpdate
	if apply == nil {
		return nil, internalErrorf("update apply service is not configured")
	}
	app, err := service.Store.GetApp(req.ID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, Errorf(ErrorNotFound, "", "managed app %q was not found", req.ID)
	}
	checks := appupdate.CheckManagedUpdates([]*domain.App{app}, nil)
	if len(checks) == 0 || checks[0].Update == nil || !checks[0].Update.Available {
		return &UpdateApplyResult{App: app}, nil
	}
	updated, err := apply(ctx, *checks[0].Update, nil)
	if err != nil {
		return nil, err
	}
	return &UpdateApplyResult{App: updated, Update: checks[0].Update}, nil
}

func (service SourceUpdateService) ApplyBatch(ctx context.Context, req UpdateApplyBatchRequest) (*UpdateApplyBatchResult, error) {
	apply := service.ApplyManagedUpdate
	if apply == nil {
		return nil, internalErrorf("update apply service is not configured")
	}
	results := appupdate.ApplyManagedUpdates(ctx, req.Pending, func(ctx context.Context, update appupdate.ManagedUpdate, reporter appupdate.ManagedApplyReporter) (*domain.App, error) {
		return apply(ctx, update, reporter)
	}, req.ReporterFor)
	return &UpdateApplyBatchResult{Results: results}, nil
}

func (service SourceUpdateService) SetSource(ctx context.Context, req UpdateSourceRequest) (*UpdateSourceResult, error) {
	_ = ctx
	if service.Store == nil {
		return nil, internalErrorf("app store is not configured")
	}
	if err := domain.ValidateUpdateSource(req.Source); err != nil {
		return nil, NewError(ErrorInvalidInput, "", err)
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
		return nil, internalErrorf("app store is not configured")
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
		return nil, internalErrorf("app store is not configured")
	}
	app, err := service.Store.GetApp(id)
	if err != nil {
		return nil, err
	}
	values := UpdateUnsetDryRunValues(id, app.Update)
	return &DryRunPlan{Action: "unset_update_source", Target: id, Values: values}, nil
}

func RequireInstallablePackage(metadata *domain.PackageMetadata) (*domain.PackageMetadata, error) {
	if metadata != nil && metadata.Installable {
		return metadata, nil
	}
	if metadata == nil {
		return nil, invalidInputErrorf("package metadata cannot be empty")
	}
	return nil, invalidInputErrorf("package is not installable: %s", strings.TrimSpace(metadata.InstallReason))
}

func removeResultFromDomain(app *domain.App, unlink bool) *RemoveResult {
	values := RemoveDryRunValues(app, unlink)
	paths, _ := values["paths"].([]string)
	action, _ := values["action"].(string)
	return &RemoveResult{
		Action: action,
		App:    appDetailsFromDomain(app),
		Unlink: unlink,
		Paths:  paths,
	}
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
