package services

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

type AddWorkflowService struct {
	Store             AppStore
	Discovery         DiscoveryService
	Installer         RemoteInstaller
	HasExtension      func(string, string) bool
	IntegrateLocalApp IntegrateFunc
	ReintegrateApp    func(context.Context, string) (*domain.App, error)
	AppImageInfo      AppImageInfoReader
	AimDir            string
	DesktopDir        string
}

func NewAddWorkflowService(service AddWorkflowService) AddWorkflowService {
	return service
}

func (service AddWorkflowService) Add(ctx context.Context, req AddRequest) (*AddResult, error) {
	if req.Target.Provider != nil {
		installReq := InstallPackageRefRequest{Ref: *req.Target.Provider, AssetPattern: req.AssetPattern, ResolveViewAmbiguity: req.ResolvePackageAmbiguity}
		if req.DryRun {
			plan, err := service.PlanPackageRefInstall(ctx, installReq)
			if err != nil {
				return nil, err
			}
			return addPlanResult(AddActionInstall, plan), nil
		}
		return service.installPackageRef(ctx, installReq)
	}

	if strings.TrimSpace(req.Target.URL) != "" {
		installReq := InstallDirectURLRequest{URL: req.Target.URL, SHA256: req.SHA256}
		if req.DryRun {
			plan, err := service.PlanDirectURLInstall(ctx, installReq)
			if err != nil {
				return nil, err
			}
			return addPlanResult(AddActionInstall, plan), nil
		}
		return service.installDirectURL(ctx, installReq)
	}

	target, err := service.ResolveIntegrateTarget(ctx, req.Target.Positional)
	if err != nil {
		return nil, err
	}
	switch target.Kind {
	case IntegrateTargetIntegrated:
		return &AddResult{Action: AddActionAlreadyIntegrated, Status: "already_integrated", App: target.App, AlreadyIntegrated: true}, nil
	case IntegrateTargetUnlinked:
		if target.App == nil {
			return nil, internalErrorf("reintegrate target missing app")
		}
		if req.DryRun {
			return &AddResult{Action: AddActionReintegrate, Status: "dry_run", App: target.App}, nil
		}
		return service.reintegrate(ctx, target.App.ID)
	case IntegrateTargetLocalFile:
		if req.DryRun {
			plan, err := service.PlanLocalIntegration(ctx, target.LocalPath)
			if err != nil {
				return nil, err
			}
			return addPlanResult(AddActionIntegrate, plan), nil
		}
		return service.addLocal(ctx, IntegrateLocalRequest{Path: target.LocalPath, ConfirmUpdateSourceReplace: req.ConfirmUpdateSourceReplace})
	default:
		return nil, internalErrorf("unknown add target kind %q", target.Kind)
	}
}

func (service AddWorkflowService) ResolveIntegrateTarget(ctx context.Context, input string) (*IntegrateTargetResult, error) {
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
			return &IntegrateTargetResult{Kind: kind, App: appDetailsFromDomain(app)}, nil
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

func (service AddWorkflowService) addLocal(ctx context.Context, req IntegrateLocalRequest) (*AddResult, error) {
	if service.IntegrateLocalApp == nil {
		return nil, internalErrorf("integrate local app service is not configured")
	}
	var prompt appintegrate.UpdateOverwritePrompt
	if req.ConfirmUpdateSourceReplace != nil {
		prompt = func(existing, incoming *domain.UpdateSource) (bool, error) {
			return req.ConfirmUpdateSourceReplace.ConfirmUpdateSourceReplace(updateSourceViewFromDomain(existing), updateSourceViewFromDomain(incoming))
		}
	}
	app, err := service.IntegrateLocalApp(ctx, req.Path, prompt)
	if err != nil {
		return nil, err
	}
	return addResultFromDomain("integrated", app), nil
}

func (service AddWorkflowService) reintegrate(ctx context.Context, id string) (*AddResult, error) {
	if service.ReintegrateApp == nil {
		return nil, internalErrorf("reintegrate app service is not configured")
	}
	app, err := service.ReintegrateApp(ctx, id)
	if err != nil {
		return nil, err
	}
	return addResultFromDomain("reintegrated", app), nil
}

func (service AddWorkflowService) installDirectURL(ctx context.Context, req InstallDirectURLRequest) (*AddResult, error) {
	if service.Installer == nil {
		return nil, internalErrorf("remote installer is not configured")
	}
	app, err := service.Installer.InstallDirectURL(ctx, req)
	if err != nil {
		return nil, err
	}
	return addResultFromDomain("installed", app), nil
}

func (service AddWorkflowService) installPackageRef(ctx context.Context, req InstallPackageRefRequest) (*AddResult, error) {
	if service.Discovery == nil {
		return nil, internalErrorf("discovery service is not configured")
	}
	if service.Installer == nil {
		return nil, internalErrorf("remote installer is not configured")
	}
	metadata, err := service.Discovery.ResolvePackage(ctx, PackageRefInfoRequest{Ref: req.Ref, AssetPattern: req.AssetPattern})
	if err != nil {
		return nil, err
	}
	metadata, err = RequireInstallablePackage(metadata)
	if err != nil {
		return nil, err
	}
	if req.ResolveViewAmbiguity != nil {
		resolved, err := req.ResolveViewAmbiguity.ResolvePackageViewAmbiguity(packageViewFromDomain(metadata))
		if err != nil {
			return nil, err
		}
		applyPackageViewSelection(metadata, resolved)
	}
	app, err := service.Installer.InstallPackageMetadata(ctx, metadata)
	if err != nil {
		return nil, err
	}
	return addResultFromDomain("installed", app), nil
}

func addResultFromDomain(status string, app *domain.App) *AddResult {
	result := &AddResult{Status: strings.TrimSpace(status), App: appDetailsFromDomain(app)}
	switch result.Status {
	case "integrated":
		result.Action = AddActionIntegrate
	case "reintegrated":
		result.Action = AddActionReintegrate
	case "installed":
		result.Action = AddActionInstall
	}
	return result
}

func addPlanResult(action AddAction, plan *DryRunPlan) *AddResult {
	result := &AddResult{Action: action, Status: "dry_run", Plan: plan}
	if plan != nil {
		result.App = plan.App
		result.Package = plan.Package
	}
	return result
}

func (service AddWorkflowService) PlanLocalIntegration(ctx context.Context, path string) (*DryRunPlan, error) {
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

func (service AddWorkflowService) PlanDirectURLInstall(ctx context.Context, req InstallDirectURLRequest) (*DryRunPlan, error) {
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

func (service AddWorkflowService) PlanPackageRefInstall(ctx context.Context, req InstallPackageRefRequest) (*DryRunPlan, error) {
	if service.Discovery == nil {
		return nil, internalErrorf("discovery service is not configured")
	}
	metadata, err := service.Discovery.ResolvePackage(ctx, PackageRefInfoRequest{Ref: req.Ref, AssetPattern: req.AssetPattern})
	if err != nil {
		return nil, err
	}
	target := FormatProviderRef(req.Ref)
	values := map[string]interface{}{
		"action":   "install",
		"target":   target,
		"provider": req.Ref,
		"metadata": packageViewFromDomain(metadata),
		"db_write": true,
	}
	return &DryRunPlan{Action: "install", Target: target, Values: values, TargetKind: "package", Package: packageViewFromDomain(metadata), DBWrite: true}, nil
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

	filter := req.Filter
	if filter == "" {
		filter = ListAll
	}

	result := &ListResult{Apps: make([]*AppDetails, 0, len(apps))}
	for _, app := range apps {
		if app == nil {
			continue
		}
		result.TotalCount++
		integrated := strings.TrimSpace(app.DesktopEntryLink) != ""
		if integrated {
			result.IntegratedCount++
		} else {
			result.UnlinkedCount++
		}
		if listFilterIncludes(filter, integrated) {
			result.Apps = append(result.Apps, appDetailsFromDomain(app))
		}
	}
	return result, nil
}

func listFilterIncludes(filter ListFilter, integrated bool) bool {
	switch filter {
	case "", ListAll:
		return true
	case ListIntegrated:
		return integrated
	case ListUnlinked:
		return !integrated
	default:
		return true
	}
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

func (service StoreInfoService) Info(ctx context.Context, req InfoRequest) (*InfoResult, error) {
	if req.Provider != nil {
		return service.packageRefInfo(ctx, PackageRefInfoRequest{Ref: *req.Provider, AssetPattern: req.AssetPattern})
	}

	input := strings.TrimSpace(req.Input)
	if input == "" {
		return nil, invalidInputErrorf("missing required argument <id|Path/To.AppImage>")
	}
	if req.ManagedOnly {
		return service.managedAppInfo(ctx, input)
	}
	if hasAppImageExtension(input) {
		return service.localAppImageInfo(ctx, input)
	}
	if looksLikeInfoRemote(input) {
		return nil, invalidInputErrorf("remote package lookups must use provider flags")
	}
	return service.managedAppInfo(ctx, input)
}

func (service StoreInfoService) managedAppInfo(ctx context.Context, id string) (*InfoResult, error) {
	_ = ctx
	if service.Store == nil {
		return nil, internalErrorf("app store is not configured")
	}
	app, err := service.Store.GetApp(id)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, Errorf(ErrorNotFound, "", "managed app %q was not found", id)
	}
	embedded, _ := service.embeddedUpdateSource(app.ExecPath)
	return &InfoResult{
		Kind:           InfoKindManagedApp,
		AppDetails:     appDetailsFromDomain(app),
		EmbeddedUpdate: updateSourceViewFromDomain(embedded),
	}, nil
}

func (service StoreInfoService) localAppImageInfo(ctx context.Context, path string) (*InfoResult, error) {
	if service.AppImage == nil {
		return nil, internalErrorf("appimage info reader is not configured")
	}
	info, err := service.AppImage.ReadAppImageInfo(ctx, path)
	if err != nil {
		return nil, err
	}
	embedded, _ := service.embeddedUpdateSource(path)
	return &InfoResult{
		Kind:           InfoKindLocalAppImage,
		AppImageInfo:   appImageInfoViewFromAppImageInfo(info),
		EmbeddedUpdate: updateSourceViewFromDomain(embedded),
	}, nil
}

func (service StoreInfoService) packageRefInfo(ctx context.Context, req PackageRefInfoRequest) (*InfoResult, error) {
	if service.Discovery == nil {
		return nil, internalErrorf("discovery service is not configured")
	}
	metadata, err := service.Discovery.ResolvePackage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &InfoResult{
		Kind:        InfoKindPackage,
		PackageView: packageViewFromDomain(metadata),
	}, nil
}

func hasAppImageExtension(input string) bool {
	return strings.EqualFold(filepath.Ext(strings.TrimSpace(input)), ".AppImage")
}

func looksLikeInfoRemote(input string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	return strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://")
}

func (service StoreInfoService) embeddedUpdateSource(path string) (*domain.UpdateSource, error) {
	return readEmbeddedUpdateSource(service.UpdateInfo, path)
}

func readEmbeddedUpdateSource(reader UpdateInfoReader, path string) (*domain.UpdateSource, error) {
	if reader == nil {
		return nil, internalErrorf("update info reader is not configured")
	}
	info, err := reader.ReadUpdateInfo(strings.TrimSpace(path))
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
	if req.DryRun {
		if service.Store == nil {
			return nil, internalErrorf("app store is not configured")
		}
		app, err := service.Store.GetApp(req.ID)
		if err != nil {
			return nil, err
		}
		return removeResultFromDomain(app, req.Unlink), nil
	}

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
	return discovery.NewService(backends...).Resolve(ctx, providerRefToDomain(req.Ref), req.AssetPattern)
}

type UpgradeWorkflowService struct {
	CheckFunc   func(context.Context, string) (*appupgrade.AimUpgradeCheckResult, error)
	UpgradeFunc func(context.Context, string) (*appupgrade.InstallerUpgradeResult, error)
}

func NewUpgradeWorkflowService(service UpgradeWorkflowService) UpgradeWorkflowService {
	return service
}

func (service UpgradeWorkflowService) Upgrade(ctx context.Context, req UpgradeRequest) (*UpgradeResult, error) {
	if service.CheckFunc == nil {
		return nil, internalErrorf("upgrade check service is not configured")
	}
	check, err := service.CheckFunc(ctx, req.CurrentVersion)
	if err != nil {
		return nil, err
	}
	result := &UpgradeResult{CurrentVersion: strings.TrimSpace(req.CurrentVersion)}
	if check != nil {
		result.CurrentVersion = strings.TrimSpace(check.CurrentVersion)
		result.LatestVersion = strings.TrimSpace(check.LatestVersion)
		if result.CurrentVersion == "" {
			result.CurrentVersion = strings.TrimSpace(req.CurrentVersion)
		}
		if check.Comparable && !check.HasUpdate {
			result.UpToDate = true
			return result, nil
		}
	}
	if req.DryRun {
		return result, nil
	}
	if service.UpgradeFunc == nil {
		return nil, internalErrorf("upgrade installer service is not configured")
	}
	installer, err := service.UpgradeFunc(ctx, req.CurrentVersion)
	if err != nil {
		return nil, err
	}
	result.Upgraded = true
	if installer != nil {
		if strings.TrimSpace(installer.PreviousVersion) != "" {
			result.CurrentVersion = strings.TrimSpace(installer.PreviousVersion)
		}
		result.InstalledVersion = strings.TrimSpace(installer.InstalledVersion)
	}
	return result, nil
}

type ManagedUpdateApplier func(context.Context, appupdate.ManagedUpdate, appupdate.ManagedApplyReporter) (*domain.App, error)
type ManagedUpdateChecker = appupdate.ManagedCheckFunc

type CheckMetadataUpdate struct {
	ID            string
	Checked       bool
	Available     bool
	Latest        string
	LastCheckedAt string
}

type SourceUpdateService struct {
	Store                AppStore
	Locker               StateLocker
	UpdateInfo           UpdateInfoReader
	CheckManagedUpdate   ManagedUpdateChecker
	LoadCheckCache       func() (*appupdate.CheckCacheFile, error)
	SaveCheckCache       func(*appupdate.CheckCacheFile) error
	PersistCheckMetadata func([]CheckMetadataUpdate) error
	InvalidateCheckCache func([]string) error
	ApplyManagedUpdate   ManagedUpdateApplier
	PersistApps          func([]*domain.App, bool) error
	PersistApp           func(*domain.App, bool) error
	RemoveApp            func(context.Context, string, bool) (*domain.App, error)
	RefreshCaches        func(context.Context)
	NowISO               func() string
}

func NewSourceUpdateService(store AppStore) SourceUpdateService {
	return SourceUpdateService{Store: store}
}

func NewSourceUpdateWorkflowService(service SourceUpdateService) SourceUpdateService {
	return service
}

func (service SourceUpdateService) Update(ctx context.Context, req UpdateRequest) (*UpdateResult, error) {
	mode := req.Mode
	if mode == "" {
		mode = UpdateModeManagedCheckApply
	}
	switch mode {
	case UpdateModeSetSource:
		return service.updateSetSource(ctx, req)
	case UpdateModeUnsetSource:
		return service.updateUnsetSource(ctx, req)
	}
	check, err := service.checkManagedUpdates(ctx, ManagedUpdateCheckRequest{
		TargetID:   req.TargetID,
		DryRun:     req.DryRun,
		UseCache:   req.UseCache,
		OnCacheHit: req.OnCacheHit,
	})
	if err != nil {
		return nil, err
	}
	result := updateResultFromManagedCheck(mode, check)
	if len(result.Pending) == 0 || mode == UpdateModeCheckOnly {
		return result, nil
	}
	if req.DryRun {
		markUpdateRows(result.Rows, "update_available", "dry_run_pending")
		return result, nil
	}

	confirmed := req.AutoApply
	if !confirmed && req.ConfirmApply != nil {
		confirmed, err = req.ConfirmApply.ConfirmUpdateApply(UpdateApplyConfirmation{TargetID: req.TargetID, Pending: result.Pending})
		if err != nil {
			return result, err
		}
	}
	if !confirmed {
		markUpdateRows(result.Rows, "update_available", "apply_skipped")
		result.ApplySkipped = true
		return result, nil
	}

	apply, err := service.applyBatchWithOptionalLock(ctx, UpdateApplyBatchRequest{Pending: check.PendingHandles, ReporterFor: req.ReporterFor})
	if apply != nil {
		result.Applied = apply.Results
		result.ApplyFailures = countApplyFailures(apply.Results)
		service.reconcileAppliedUpdateRows(result.Rows, apply.Results)
	}
	if err != nil {
		return result, err
	}
	if result.ApplyFailures > 0 {
		return result, Errorf(ErrorUnavailable, "", "%d update(s) failed", result.ApplyFailures)
	}
	return result, nil
}

func (service SourceUpdateService) updateSetSource(ctx context.Context, req UpdateRequest) (*UpdateResult, error) {
	id := strings.TrimSpace(req.TargetID)
	if id == "" {
		return nil, invalidInputErrorf("missing update source target id")
	}
	app, err := service.getManagedApp(id)
	if err != nil {
		return nil, err
	}
	current := updateSourceViewFromDomain(app.Update)
	incoming := req.Source
	noEmbedded := false
	if req.UseEmbeddedSource {
		source, err := readEmbeddedUpdateSource(service.UpdateInfo, app.ExecPath)
		if err != nil {
			noEmbedded = true
		} else {
			incoming = updateSourceInputFromDomain(source)
		}
	}
	incomingDomain := updateSourceInputToDomain(incoming)
	incomingView := updateSourceViewFromDomain(incomingDomain)
	if noEmbedded && updateSourceViewIsNone(current) {
		return &UpdateResult{Mode: UpdateModeSetSource, NoEmbeddedSource: true, SourceUnchanged: true, SourceChange: &UpdateSourceChangeView{ID: id, Current: current}}, nil
	}
	if noEmbedded {
		return service.updateUnsetSource(ctx, UpdateRequest{TargetID: id, Mode: UpdateModeUnsetSource, DryRun: req.DryRun, ConfirmSourceUnset: req.ConfirmSourceUnset, UseEmbeddedSource: true})
	}
	if err := domain.ValidateUpdateSource(incomingDomain); err != nil {
		return nil, NewError(ErrorInvalidInput, "", err)
	}
	change := &UpdateSourceChangeView{ID: id, Current: current, Incoming: incomingView}
	if !updateSourceViewIsNone(current) && !updateSourceViewsEqual(current, incomingView) && req.ConfirmSourceReplace != nil {
		confirmed, err := req.ConfirmSourceReplace.ConfirmUpdateSourceReplace(current, incomingView)
		if err != nil {
			return &UpdateResult{Mode: UpdateModeSetSource, SourceChange: change}, err
		}
		if !confirmed {
			return &UpdateResult{Mode: UpdateModeSetSource, SourceChange: change, SourceUnchanged: true}, nil
		}
	}
	if req.DryRun {
		plan, err := service.planSetSource(ctx, updateSourceRequest{ID: id, Source: incoming})
		if err != nil {
			return nil, err
		}
		return &UpdateResult{Mode: UpdateModeSetSource, Plan: plan, SourceChange: plan.UpdateSourceChange}, nil
	}
	var sourceResult *UpdateSourceResult
	err = service.withOptionalWriteLock(func() error {
		var setErr error
		sourceResult, setErr = service.setSource(ctx, updateSourceRequest{ID: id, Source: incoming})
		return setErr
	})
	if err != nil {
		return nil, err
	}
	change.Changed = true
	return &UpdateResult{Mode: UpdateModeSetSource, Source: sourceResult, SourceChange: change}, nil
}

func (service SourceUpdateService) updateUnsetSource(ctx context.Context, req UpdateRequest) (*UpdateResult, error) {
	id := strings.TrimSpace(req.TargetID)
	if id == "" {
		return nil, invalidInputErrorf("missing update source target id")
	}
	app, err := service.getManagedApp(id)
	if err != nil {
		return nil, err
	}
	current := updateSourceViewFromDomain(app.Update)
	change := &UpdateSourceChangeView{ID: id, Current: current}
	if updateSourceViewIsNone(current) {
		return &UpdateResult{Mode: UpdateModeUnsetSource, Source: &UpdateSourceResult{ID: id, Source: current, Changed: false}, SourceChange: change, SourceUnchanged: true, NoEmbeddedSource: req.UseEmbeddedSource}, nil
	}
	if req.DryRun {
		plan, err := service.planUnsetSource(ctx, id)
		if err != nil {
			return nil, err
		}
		return &UpdateResult{Mode: UpdateModeUnsetSource, Plan: plan, SourceChange: plan.UpdateSourceChange, NoEmbeddedSource: req.UseEmbeddedSource}, nil
	}
	if req.ConfirmSourceUnset != nil {
		confirmed, err := req.ConfirmSourceUnset.ConfirmUpdateSourceUnset(current)
		if err != nil {
			return &UpdateResult{Mode: UpdateModeUnsetSource, SourceChange: change, NoEmbeddedSource: req.UseEmbeddedSource}, err
		}
		if !confirmed {
			return &UpdateResult{Mode: UpdateModeUnsetSource, SourceChange: change, SourceUnchanged: true, NoEmbeddedSource: req.UseEmbeddedSource}, nil
		}
	}
	var sourceResult *UpdateSourceResult
	err = service.withOptionalWriteLock(func() error {
		var unsetErr error
		sourceResult, unsetErr = service.unsetSource(ctx, id)
		return unsetErr
	})
	if err != nil {
		return nil, err
	}
	change.Changed = true
	return &UpdateResult{Mode: UpdateModeUnsetSource, Source: sourceResult, SourceChange: change, NoEmbeddedSource: req.UseEmbeddedSource}, nil
}

func (service SourceUpdateService) getManagedApp(id string) (*domain.App, error) {
	if service.Store == nil {
		return nil, internalErrorf("app store is not configured")
	}
	app, err := service.Store.GetApp(id)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, Errorf(ErrorNotFound, "", "managed app %q was not found", id)
	}
	return app, nil
}

func (service SourceUpdateService) withOptionalWriteLock(fn func() error) error {
	if service.Locker == nil {
		return fn()
	}
	return service.Locker.WithWriteLock(fn)
}

func updateSourceViewIsNone(update *UpdateSourceView) bool {
	return update == nil || strings.TrimSpace(update.Kind) == "" || strings.TrimSpace(update.Kind) == UpdateKindNone
}

func updateSourceViewsEqual(left, right *UpdateSourceView) bool {
	if updateSourceViewIsNone(left) && updateSourceViewIsNone(right) {
		return true
	}
	if updateSourceViewIsNone(left) || updateSourceViewIsNone(right) {
		return false
	}
	if strings.TrimSpace(left.Kind) != strings.TrimSpace(right.Kind) {
		return false
	}
	switch strings.TrimSpace(left.Kind) {
	case UpdateKindZsync:
		return left.Zsync != nil && right.Zsync != nil && strings.TrimSpace(left.Zsync.UpdateInfo) == strings.TrimSpace(right.Zsync.UpdateInfo) && strings.TrimSpace(left.Zsync.Transport) == strings.TrimSpace(right.Zsync.Transport)
	case UpdateKindGitHubRelease:
		return left.GitHubRelease != nil && right.GitHubRelease != nil && strings.TrimSpace(left.GitHubRelease.Repo) == strings.TrimSpace(right.GitHubRelease.Repo) && strings.TrimSpace(left.GitHubRelease.Asset) == strings.TrimSpace(right.GitHubRelease.Asset)
	default:
		return true
	}
}

func updateResultFromManagedCheck(mode UpdateMode, check *ManagedUpdateCheckResult) *UpdateResult {
	result := &UpdateResult{Mode: mode}
	if check == nil {
		return result
	}
	result.Rows = append(result.Rows, check.Rows...)
	result.Pending = append(result.Pending, check.Pending...)
	result.CheckFailures = check.CheckFailures
	result.Failures = append(result.Failures, check.Failures...)
	return result
}

func markUpdateRows(rows []ManagedUpdateCheckRow, from, to string) {
	for idx := range rows {
		if rows[idx].Status == from {
			rows[idx].Status = to
		}
	}
}

func countApplyFailures(results []ManagedApplyResultView) int {
	failures := 0
	for _, result := range results {
		if strings.TrimSpace(result.Error) != "" {
			failures++
		}
	}
	return failures
}

func (service SourceUpdateService) applyBatchWithOptionalLock(ctx context.Context, req UpdateApplyBatchRequest) (*UpdateApplyBatchResult, error) {
	if service.Locker == nil {
		return service.applyBatch(ctx, req)
	}
	var result *UpdateApplyBatchResult
	var applyErr error
	err := service.Locker.WithWriteLock(func() error {
		result, applyErr = service.applyBatch(ctx, req)
		return nil
	})
	if err != nil {
		return result, err
	}
	return result, applyErr
}

func (service SourceUpdateService) reconcileAppliedUpdateRows(rows []ManagedUpdateCheckRow, results []ManagedApplyResultView) {
	rowIndexByID := make(map[string]int, len(rows))
	for idx, row := range rows {
		if row.App != nil && strings.TrimSpace(row.App.ID) != "" {
			rowIndexByID[strings.TrimSpace(row.App.ID)] = idx
		}
	}
	for _, apply := range results {
		if strings.TrimSpace(apply.Error) != "" || apply.UpdatedApp == nil {
			continue
		}
		idx, ok := rowIndexByID[strings.TrimSpace(apply.UpdatedApp.ID)]
		if !ok {
			continue
		}
		rows[idx].Status = "updated"
		if rows[idx].App != nil {
			rows[idx].App.Version = strings.TrimSpace(apply.UpdatedApp.Version)
		}
	}
}

func (service SourceUpdateService) managedApps(ctx context.Context, targetID string) ([]*domain.App, error) {
	_ = ctx
	if service.Store == nil {
		return nil, internalErrorf("app store is not configured")
	}
	if strings.TrimSpace(targetID) != "" {
		app, err := service.Store.GetApp(targetID)
		if err != nil {
			return nil, err
		}
		if app == nil {
			return nil, Errorf(ErrorNotFound, "", "managed app %q was not found", targetID)
		}
		return []*domain.App{app}, nil
	}
	all, err := service.Store.GetAllApps()
	if err != nil {
		return nil, err
	}
	apps := make([]*domain.App, 0, len(all))
	for _, app := range all {
		if app != nil {
			apps = append(apps, app)
		}
	}
	sort.SliceStable(apps, func(i, j int) bool {
		if apps[i] == nil {
			return false
		}
		if apps[j] == nil {
			return true
		}
		return strings.TrimSpace(apps[i].ID) < strings.TrimSpace(apps[j].ID)
	})
	return apps, nil
}

func (service SourceUpdateService) checkManagedUpdates(ctx context.Context, req ManagedUpdateCheckRequest) (*ManagedUpdateCheckResult, error) {
	apps, err := service.managedApps(ctx, req.TargetID)
	if err != nil {
		return nil, err
	}

	cache, err := service.loadManagedUpdateCache(req)
	if err != nil {
		return nil, err
	}

	checkResults := service.runManagedChecksWithCache(apps, cache, req)
	checkedAt := service.nowISO()
	metadataUpdates := make([]CheckMetadataUpdate, 0, len(checkResults))
	result := &ManagedUpdateCheckResult{
		Rows:           make([]ManagedUpdateCheckRow, 0, len(checkResults)),
		Pending:        make([]ManagedUpdateView, 0),
		PendingHandles: make([]ManagedUpdateHandle, 0),
		Failures:       make([]ManagedCheckFailureView, 0),
	}

	for idx, checkResult := range checkResults {
		app := checkResult.App
		update := checkResult.Update
		if checkResult.Error != nil {
			if !req.DryRun && app != nil {
				metadataUpdates = append(metadataUpdates, CheckMetadataUpdate{
					ID:            app.ID,
					Checked:       false,
					Available:     app.UpdateAvailable,
					Latest:        app.LatestVersion,
					LastCheckedAt: checkedAt,
				})
			}
			result.Rows = append(result.Rows, ManagedUpdateCheckRow{App: appSummaryFromDomain(app), Update: managedUpdateViewFromAppUpdate(update), Status: "check_failed", CheckedAt: checkedAt, Error: checkResult.Error.Error()})
			result.CheckFailures++
			result.Failures = append(result.Failures, ManagedCheckFailureView{AppID: appID(app), Reason: checkResult.Error.Error()})
			if strings.TrimSpace(req.TargetID) != "" {
				break
			}
			continue
		}

		if !req.DryRun && app != nil && cache != nil {
			appupdate.SetCachedManagedUpdate(cache, app, appupdate.ManagedCheckCacheKey(app, idx), update, checkedAt)
		}

		if update == nil {
			status := "no_update_information"
			if app == nil || app.Update == nil || app.Update.Kind == domain.UpdateNone {
				status = "no_update_source"
			}
			result.Rows = append(result.Rows, ManagedUpdateCheckRow{App: appSummaryFromDomain(app), Status: status, CheckedAt: checkedAt})
			continue
		}

		if !req.DryRun && app != nil {
			metadataUpdates = append(metadataUpdates, CheckMetadataUpdate{
				ID:            app.ID,
				Checked:       true,
				Available:     update.Available,
				Latest:        update.Latest,
				LastCheckedAt: checkedAt,
			})
		}

		status := "up_to_date"
		if strings.TrimSpace(update.URL) != "" {
			status = "update_available"
			if handle := managedUpdateHandleFromAppUpdate(update); handle != nil {
				result.Pending = append(result.Pending, handle.View)
				result.PendingHandles = append(result.PendingHandles, *handle)
			}
		}
		result.Rows = append(result.Rows, ManagedUpdateCheckRow{App: appSummaryFromDomain(app), Update: managedUpdateViewFromAppUpdate(update), Status: status, CheckedAt: checkedAt})
	}

	if !req.DryRun {
		if err := service.persistCheckMetadata(metadataUpdates); err != nil {
			return result, err
		}
		if cache != nil && service.SaveCheckCache != nil {
			if err := service.SaveCheckCache(cache); err != nil {
				return result, err
			}
		}
	}
	return result, nil
}

func (service SourceUpdateService) loadManagedUpdateCache(req ManagedUpdateCheckRequest) (*appupdate.CheckCacheFile, error) {
	if req.DryRun || !req.UseCache || service.LoadCheckCache == nil {
		return nil, nil
	}
	return service.LoadCheckCache()
}

func (service SourceUpdateService) runManagedChecksWithCache(apps []*domain.App, cache *appupdate.CheckCacheFile, req ManagedUpdateCheckRequest) []appupdate.ManagedCheckResult {
	if cache == nil {
		return appupdate.CheckManagedUpdates(apps, service.CheckManagedUpdate)
	}
	results := make([]appupdate.ManagedCheckResult, len(apps))
	toCheck := make([]*domain.App, 0, len(apps))
	toCheckIndices := make([]int, 0, len(apps))
	for idx, app := range apps {
		key := appupdate.ManagedCheckCacheKey(app, idx)
		if cached, ok := appupdate.CachedManagedUpdateForApp(cache, app, key, time.Now(), appupdate.DefaultCheckCacheTTL); ok {
			results[idx] = appupdate.ManagedCheckResult{App: app, Update: cached}
			if req.OnCacheHit != nil && app != nil {
				req.OnCacheHit(strings.TrimSpace(app.ID))
			}
			continue
		}
		toCheck = append(toCheck, app)
		toCheckIndices = append(toCheckIndices, idx)
	}
	fresh := appupdate.CheckManagedUpdates(toCheck, service.CheckManagedUpdate)
	for idx, result := range fresh {
		results[toCheckIndices[idx]] = result
	}
	return results
}

func (service SourceUpdateService) persistCheckMetadata(updates []CheckMetadataUpdate) error {
	if len(updates) == 0 || service.PersistCheckMetadata == nil {
		return nil
	}
	return service.PersistCheckMetadata(updates)
}

func (service SourceUpdateService) nowISO() string {
	if service.NowISO != nil {
		return strings.TrimSpace(service.NowISO())
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func appID(app *domain.App) string {
	if app == nil {
		return ""
	}
	return strings.TrimSpace(app.ID)
}

func (service SourceUpdateService) applyBatch(ctx context.Context, req UpdateApplyBatchRequest) (*UpdateApplyBatchResult, error) {
	apply := service.ApplyManagedUpdate
	if apply == nil {
		return nil, internalErrorf("update apply service is not configured")
	}
	pending := make([]appupdate.ManagedUpdate, 0, len(req.Pending))
	for _, handle := range req.Pending {
		pending = append(pending, handle.update)
	}
	applyResults := appupdate.ApplyManagedUpdates(ctx, pending, func(ctx context.Context, update appupdate.ManagedUpdate, reporter appupdate.ManagedApplyReporter) (*domain.App, error) {
		return apply(ctx, update, reporter)
	}, func(index, total int, update appupdate.ManagedUpdate) appupdate.ManagedApplyReporter {
		if req.ReporterFor == nil {
			return nil
		}
		view := managedUpdateViewFromAppUpdate(&update)
		if view == nil {
			view = &ManagedUpdateView{}
		}
		return appupdate.WithManagedApplyEventDefaults(req.ReporterFor(index, total, *view), index, total, update)
	})
	results := make([]ManagedApplyResultView, 0, len(applyResults))
	appliedApps := make([]*domain.App, 0, len(applyResults))
	for _, result := range applyResults {
		results = append(results, managedApplyResultViewFromAppUpdate(result))
		if result.Error == nil && result.UpdatedApp != nil {
			appliedApps = append(appliedApps, result.UpdatedApp)
		}
	}
	if len(appliedApps) > 0 && service.RefreshCaches != nil {
		service.RefreshCaches(ctx)
	}
	if err := service.persistAppliedApps(ctx, appliedApps); err != nil {
		return &UpdateApplyBatchResult{Results: results}, err
	}
	if err := service.invalidateAppliedUpdateCache(appliedApps); err != nil {
		return &UpdateApplyBatchResult{Results: results}, err
	}
	return &UpdateApplyBatchResult{Results: results}, nil
}

func (service SourceUpdateService) invalidateAppliedUpdateCache(apps []*domain.App) error {
	if len(apps) == 0 || service.InvalidateCheckCache == nil {
		return nil
	}
	ids := make([]string, 0, len(apps))
	for _, app := range apps {
		if id := appID(app); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return service.InvalidateCheckCache(ids)
}

func (service SourceUpdateService) persistAppliedApps(ctx context.Context, apps []*domain.App) error {
	if len(apps) == 0 {
		return nil
	}
	if service.PersistApps != nil {
		if err := service.PersistApps(apps, true); err == nil {
			return service.cleanupReplacedApps(ctx, apps)
		}
	}
	if service.PersistApp == nil {
		return internalErrorf("app persistence service is not configured")
	}
	persistedApps := make([]*domain.App, 0, len(apps))
	fallbackErrors := make([]string, 0)
	for _, app := range apps {
		if app == nil {
			continue
		}
		if err := service.PersistApp(app, true); err != nil {
			fallbackErrors = append(fallbackErrors, strings.TrimSpace(app.ID)+": "+err.Error())
			continue
		}
		persistedApps = append(persistedApps, app)
	}
	if len(fallbackErrors) > 0 {
		return fmt.Errorf("failed to persist applied updates: %s", strings.Join(fallbackErrors, "; "))
	}
	return service.cleanupReplacedApps(ctx, persistedApps)
}

func (service SourceUpdateService) cleanupReplacedApps(ctx context.Context, apps []*domain.App) error {
	if service.RemoveApp == nil {
		return nil
	}
	replaced := map[string]struct{}{}
	for _, app := range apps {
		if app == nil {
			continue
		}
		replacesID := strings.TrimSpace(app.ReplacesID)
		if replacesID == "" || replacesID == strings.TrimSpace(app.ID) {
			continue
		}
		if _, seen := replaced[replacesID]; seen {
			continue
		}
		replaced[replacesID] = struct{}{}
		if _, err := service.RemoveApp(ctx, replacesID, false); err != nil {
			return fmt.Errorf("failed to remove superseded app %s: %w", replacesID, err)
		}
	}
	return nil
}

func (service SourceUpdateService) setSource(ctx context.Context, req updateSourceRequest) (*UpdateSourceResult, error) {
	_ = ctx
	if service.Store == nil {
		return nil, internalErrorf("app store is not configured")
	}
	source := updateSourceInputToDomain(req.Source)
	if err := domain.ValidateUpdateSource(source); err != nil {
		return nil, NewError(ErrorInvalidInput, "", err)
	}
	app, err := service.Store.GetApp(req.ID)
	if err != nil {
		return nil, err
	}
	app.Update = source
	if err := service.Store.UpdateApp(app); err != nil {
		return nil, err
	}
	return &UpdateSourceResult{ID: req.ID, Source: updateSourceViewFromDomain(source), Changed: true}, nil
}

func (service SourceUpdateService) unsetSource(ctx context.Context, id string) (*UpdateSourceResult, error) {
	return service.setSource(ctx, updateSourceRequest{ID: id, Source: &UpdateSourceInput{Kind: UpdateKindNone}})
}

func (service SourceUpdateService) planSetSource(ctx context.Context, req updateSourceRequest) (*DryRunPlan, error) {
	_ = ctx
	if service.Store == nil {
		return nil, internalErrorf("app store is not configured")
	}
	app, err := service.Store.GetApp(req.ID)
	if err != nil {
		return nil, err
	}
	source := updateSourceInputToDomain(req.Source)
	values := UpdateSetDryRunValues(req.ID, app.Update, source)
	return &DryRunPlan{
		Action:     "set_update_source",
		Target:     req.ID,
		Values:     values,
		TargetKind: "managed_app",
		UpdateSourceChange: &UpdateSourceChangeView{
			ID:       strings.TrimSpace(req.ID),
			Current:  updateSourceViewFromDomain(app.Update),
			Incoming: updateSourceViewFromDomain(source),
		},
		DBWrite: true,
	}, nil
}

func (service SourceUpdateService) planUnsetSource(ctx context.Context, id string) (*DryRunPlan, error) {
	_ = ctx
	if service.Store == nil {
		return nil, internalErrorf("app store is not configured")
	}
	app, err := service.Store.GetApp(id)
	if err != nil {
		return nil, err
	}
	values := UpdateUnsetDryRunValues(id, app.Update)
	return &DryRunPlan{
		Action:     "unset_update_source",
		Target:     id,
		Values:     values,
		TargetKind: "managed_app",
		UpdateSourceChange: &UpdateSourceChangeView{
			ID:      strings.TrimSpace(id),
			Current: updateSourceViewFromDomain(app.Update),
		},
		DBWrite: true,
	}, nil
}

func ParseGitHubProviderRef(value string) (ProviderRef, error) {
	ref, err := discovery.ParseGitHubRepoValue(value)
	if err != nil {
		return ProviderRef{}, err
	}
	return providerRefFromDomain(ref), nil
}

func ParsePackageProviderRefURL(value string) (ProviderRef, error) {
	ref, err := discovery.ParsePackageRefURL(value)
	if err != nil {
		return ProviderRef{}, err
	}
	return providerRefFromDomain(ref), nil
}

func FormatProviderRef(ref ProviderRef) string {
	value := strings.TrimSpace(ref.Ref)
	if value == "" {
		return ""
	}
	switch strings.TrimSpace(ref.Provider) {
	case ProviderGitHub:
		return "GitHub " + value
	default:
		return value
	}
}

func NormalizeComparableVersion(value string) string {
	return domain.NormalizeComparableVersion(value)
}

func ManagedUpdateDownloadFilename(assetName, downloadURL string) string {
	return appupdate.ManagedUpdateDownloadFilename(assetName, downloadURL)
}

func applyPackageViewSelection(metadata *domain.PackageMetadata, view *PackageView) {
	if metadata == nil || view == nil {
		return
	}
	if strings.TrimSpace(view.AssetName) != "" {
		metadata.AssetName = strings.TrimSpace(view.AssetName)
	}
	if strings.TrimSpace(view.DownloadURL) != "" {
		metadata.DownloadURL = strings.TrimSpace(view.DownloadURL)
	}
	metadata.AssetAmbiguous = view.AssetAmbiguous
	metadata.AssetReason = strings.TrimSpace(view.AssetReason)
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
