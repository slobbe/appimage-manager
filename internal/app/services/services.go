package services

import (
	"context"

	appimageapp "github.com/slobbe/appimage-manager/internal/app/appimage"
	"github.com/slobbe/appimage-manager/internal/app/update"
	"github.com/slobbe/appimage-manager/internal/app/upgrade"
	"github.com/slobbe/appimage-manager/internal/domain"
)

type AddService interface {
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
	Check(ctx context.Context, req UpdateCheckRequest) (*UpdateCheckResult, error)
	Apply(ctx context.Context, req UpdateApplyRequest) (*UpdateApplyResult, error)
	SetSource(ctx context.Context, req UpdateSourceRequest) (*UpdateSourceResult, error)
	UnsetSource(ctx context.Context, id string) (*UpdateSourceResult, error)
	PlanSetSource(ctx context.Context, req UpdateSourceRequest) (*DryRunPlan, error)
	PlanUnsetSource(ctx context.Context, id string) (*DryRunPlan, error)
}

type UpgradeService interface {
	Check(ctx context.Context, currentVersion string) (*upgrade.AimUpgradeCheckResult, error)
	Upgrade(ctx context.Context, currentVersion string) (*upgrade.InstallerUpgradeResult, error)
}

type DiscoveryService interface {
	ResolvePackage(ctx context.Context, req PackageRefInfoRequest) (*domain.PackageMetadata, error)
}

type StateLocker interface {
	WithWriteLock(fn func() error) error
}

type Clock interface {
	NowISO() string
}

type IntegrateLocalRequest struct {
	Path                       string
	ConfirmUpdateSourceReplace UpdateSourceReplaceConfirmer
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

type UpdateCheckRequest struct {
	ID        string
	CheckAll  bool
	UseCache  bool
	CheckOnly bool
}

type UpdateApplyRequest struct {
	ID string
}

type UpdateSourceRequest struct {
	ID     string
	Source *domain.UpdateSource
}

type AddResult struct {
	Status string
	App    *domain.App
}

type ListResult struct {
	Apps []*domain.App
}

type InfoResult struct {
	Kind           InfoKind
	App            *domain.App
	AppImage       *appimageapp.AppInfo
	Package        *domain.PackageMetadata
	EmbeddedUpdate *domain.UpdateSource
}

type RemoveResult struct {
	App    *domain.App
	Unlink bool
	Paths  []string
}

type UpdateCheckResult struct {
	Apps []ManagedUpdateStatus
}

type UpdateApplyResult struct {
	App    *domain.App
	Update *update.ManagedUpdate
}

type UpdateSourceResult struct {
	ID      string
	Source  *domain.UpdateSource
	Changed bool
}

type ManagedUpdateStatus struct {
	App    *domain.App
	Update *update.ManagedUpdate
	Error  error
}

type DryRunPlan struct {
	Action string
	Target string
	Values map[string]interface{}
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
