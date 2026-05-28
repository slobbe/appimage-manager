package services

import (
	"context"

	"github.com/slobbe/appimage-manager/internal/app/update"
	"github.com/slobbe/appimage-manager/internal/app/upgrade"
	"github.com/slobbe/appimage-manager/internal/domain"
)

type AddService interface {
	ResolveIntegrateTarget(ctx context.Context, input string) (*IntegrateTargetResult, error)
	IntegrateLocal(ctx context.Context, req IntegrateLocalRequest) (*AddResult, error)
	Reintegrate(ctx context.Context, id string) (*AddResult, error)
	InstallDirectURL(ctx context.Context, req InstallDirectURLRequest) (*AddResult, error)
	InstallPackageRef(ctx context.Context, req InstallPackageRefRequest) (*AddResult, error)
	PlanLocalIntegration(ctx context.Context, path string) (*DryRunPlan, error)
	PlanDirectURLInstall(ctx context.Context, req InstallDirectURLRequest) (*DryRunPlan, error)
	PlanPackageRefInstall(ctx context.Context, req InstallPackageRefRequest) (*DryRunPlan, error)
}

type ListService interface {
	List(ctx context.Context, req ListRequest) (*ListResult, error)
}

type InfoService interface {
	ManagedAppInfo(ctx context.Context, id string) (*InfoResult, error)
	LocalAppImageInfo(ctx context.Context, path string) (*InfoResult, error)
	PackageRefInfo(ctx context.Context, req PackageRefInfoRequest) (*InfoResult, error)
}

type RemoveService interface {
	Remove(ctx context.Context, req RemoveRequest) (*RemoveResult, error)
	PlanRemove(ctx context.Context, req RemoveRequest) (*DryRunPlan, error)
}

type UpdateService interface {
	ManagedApps(ctx context.Context, targetID string) ([]*domain.App, error)
	ApplyBatch(ctx context.Context, req UpdateApplyBatchRequest) (*UpdateApplyBatchResult, error)
	EmbeddedSource(ctx context.Context, id string) (*UpdateSourceResult, error)
	SetSource(ctx context.Context, req UpdateSourceRequest) (*UpdateSourceResult, error)
	SetEmbeddedSource(ctx context.Context, id string) (*UpdateSourceResult, error)
	UnsetSource(ctx context.Context, id string) (*UpdateSourceResult, error)
	PlanSetSource(ctx context.Context, req UpdateSourceRequest) (*DryRunPlan, error)
	PlanSetEmbeddedSource(ctx context.Context, id string) (*DryRunPlan, error)
	PlanUnsetSource(ctx context.Context, id string) (*DryRunPlan, error)
}

type AimUpgradeCheckResult = upgrade.AimUpgradeCheckResult

type InstallerUpgradeResult = upgrade.InstallerUpgradeResult

type UpgradeService interface {
	Check(ctx context.Context, currentVersion string) (*AimUpgradeCheckResult, error)
	Upgrade(ctx context.Context, currentVersion string) (*InstallerUpgradeResult, error)
}

type DiscoveryService interface {
	ResolvePackage(ctx context.Context, req PackageRefInfoRequest) (*domain.PackageMetadata, error)
}

type StateLocker interface {
	WithWriteLock(fn func() error) error
}

type IntegrateLocalRequest struct {
	Path                       string
	ConfirmUpdateSourceReplace UpdateSourceReplaceConfirmer
}

type IntegrateTargetKind string

const (
	IntegrateTargetLocalFile  IntegrateTargetKind = "local_file"
	IntegrateTargetUnlinked   IntegrateTargetKind = "unlinked"
	IntegrateTargetIntegrated IntegrateTargetKind = "integrated"
)

type IntegrateTargetResult struct {
	Kind      IntegrateTargetKind
	App       *AppDetails
	LocalPath string
}

type InstallDirectURLRequest struct {
	URL    string
	SHA256 string
}

type InstallPackageRefRequest struct {
	Ref              domain.PackageRef
	AssetPattern     string
	ResolveMetadata  func(context.Context, domain.PackageRef, string) (*domain.PackageMetadata, error)
	ResolveAmbiguity PackageAmbiguityResolver
	InstallPackage   func(context.Context, *domain.PackageMetadata) (*domain.App, error)
}

type PackageRefInfoRequest struct {
	Ref          domain.PackageRef
	AssetPattern string
}

type ListRequest struct {
	IncludeIntegrated bool
	IncludeUnlinked   bool
}

type RemoveRequest struct {
	ID     string
	Unlink bool
}

type UpdateApplyBatchRequest struct {
	Pending     []update.ManagedUpdate
	ReporterFor update.ManagedApplyReporterFactory
}

type UpdateSourceRequest struct {
	ID     string
	Source *domain.UpdateSource
}

type AddResult struct {
	Status string
	App    *AppDetails
}

type ListResult struct {
	Apps []*AppDetails
}

type InfoResult struct {
	Kind           InfoKind
	AppDetails     *AppDetails
	AppImageInfo   *AppImageInfoView
	PackageView    *PackageView
	EmbeddedUpdate *UpdateSourceView
}

type RemoveResult struct {
	Action string
	App    *AppDetails
	Unlink bool
	Paths  []string
}

type UpdateApplyBatchResult struct {
	Results []ManagedApplyResultView
}

type UpdateSourceResult struct {
	ID      string
	Source  *UpdateSourceView
	Changed bool
}

type DryRunPlan struct {
	Action string
	Target string
	Values map[string]interface{}

	TargetKind         string
	App                *AppDetails
	Package            *PackageView
	AppImage           *AppImageInfoView
	UpdateSourceChange *UpdateSourceChangeView
	Paths              []string
	DBWrite            bool
}

type InfoKind string

const (
	InfoKindManagedApp    InfoKind = "managed_app"
	InfoKindLocalAppImage InfoKind = "local_appimage"
	InfoKindPackage       InfoKind = "package_metadata"
)

type UpdateSourceReplaceConfirmer interface {
	ConfirmUpdateSourceReplace(existing, incoming *domain.UpdateSource) (bool, error)
}

type PackageAmbiguityResolver interface {
	ResolvePackageAmbiguity(metadata *domain.PackageMetadata) (*domain.PackageMetadata, error)
}
