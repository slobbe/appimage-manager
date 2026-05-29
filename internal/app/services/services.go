package services

import (
	"context"

	"github.com/slobbe/appimage-manager/internal/app/update"
	"github.com/slobbe/appimage-manager/internal/app/upgrade"
	"github.com/slobbe/appimage-manager/internal/domain"
)

type AddService interface {
	Add(ctx context.Context, req AddRequest) (*AddResult, error)
}

type ListService interface {
	List(ctx context.Context, req ListRequest) (*ListResult, error)
}

type InfoService interface {
	Info(ctx context.Context, req InfoRequest) (*InfoResult, error)
}

type RemoveService interface {
	Remove(ctx context.Context, req RemoveRequest) (*RemoveResult, error)
}

type UpdateService interface {
	Update(ctx context.Context, req UpdateRequest) (*UpdateResult, error)
}

type AimUpgradeCheckResult = upgrade.AimUpgradeCheckResult

type InstallerUpgradeResult = upgrade.InstallerUpgradeResult

type UpgradeService interface {
	Upgrade(ctx context.Context, req UpgradeRequest) (*UpgradeResult, error)
}

type DiscoveryService interface {
	ResolvePackage(ctx context.Context, req PackageRefInfoRequest) (*domain.PackageMetadata, error)
}

type RemoteInstaller interface {
	InstallDirectURL(ctx context.Context, req InstallDirectURLRequest) (*domain.App, error)
	InstallPackageMetadata(ctx context.Context, metadata *domain.PackageMetadata) (*domain.App, error)
}

type StateLocker interface {
	WithWriteLock(fn func() error) error
}

type AddRequest struct {
	Target AddTargetInput

	DryRun bool

	SHA256       string
	AssetPattern string

	ConfirmUpdateSourceReplace UpdateSourceReplaceConfirmer
	ResolvePackageAmbiguity    PackageViewAmbiguityResolver
}

type AddTargetInput struct {
	Positional string
	URL        string
	Provider   *ProviderRef
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
	Ref                  ProviderRef
	AssetPattern         string
	ResolveViewAmbiguity PackageViewAmbiguityResolver
}

type PackageRefInfoRequest struct {
	Ref          ProviderRef
	AssetPattern string
}

type InfoRequest struct {
	Input        string
	Provider     *ProviderRef
	AssetPattern string
	ManagedOnly  bool
}

type ListRequest struct {
	Filter ListFilter
}

type ListFilter string

const (
	ListAll        ListFilter = "all"
	ListIntegrated ListFilter = "integrated"
	ListUnlinked   ListFilter = "unlinked"
)

type RemoveRequest struct {
	ID     string
	Unlink bool
	DryRun bool
}

type UpgradeRequest struct {
	CurrentVersion string
	DryRun         bool
}

type UpdateMode string

const (
	UpdateModeManagedCheckApply UpdateMode = "managed_check_apply"
	UpdateModeCheckOnly         UpdateMode = "check_only"
	UpdateModeSetSource         UpdateMode = "set_source"
	UpdateModeUnsetSource       UpdateMode = "unset_source"
)

type UpdateRequest struct {
	TargetID  string
	Mode      UpdateMode
	DryRun    bool
	AutoApply bool
	UseCache  bool

	Source            *UpdateSourceInput
	UseEmbeddedSource bool

	OnCacheHit           func(appID string)
	ConfirmApply         UpdateApplyConfirmer
	ConfirmSourceReplace UpdateSourceReplaceConfirmer
	ConfirmSourceUnset   UpdateSourceUnsetConfirmer
	ReporterFor          ManagedApplyReporterFactory
}

type UpdateApplyConfirmation struct {
	TargetID string
	Pending  []ManagedUpdateView
}

type UpdateApplyConfirmer interface {
	ConfirmUpdateApply(UpdateApplyConfirmation) (bool, error)
}

type UpdateSourceUnsetConfirmer interface {
	ConfirmUpdateSourceUnset(current *UpdateSourceView) (bool, error)
}

type ManagedUpdateCheckRequest struct {
	TargetID   string
	DryRun     bool
	UseCache   bool
	OnCacheHit func(appID string)
}

type ManagedApplyStage = update.ManagedApplyStage

const (
	ManagedApplyStageQueued    ManagedApplyStage = update.ManagedApplyStageQueued
	ManagedApplyStageZsync     ManagedApplyStage = update.ManagedApplyStageZsync
	ManagedApplyStageDownload  ManagedApplyStage = update.ManagedApplyStageDownload
	ManagedApplyStageVerify    ManagedApplyStage = update.ManagedApplyStageVerify
	ManagedApplyStageIntegrate ManagedApplyStage = update.ManagedApplyStageIntegrate
	ManagedApplyStageDone      ManagedApplyStage = update.ManagedApplyStageDone
	ManagedApplyStageFailed    ManagedApplyStage = update.ManagedApplyStageFailed
)

type ManagedApplyEvent = update.ManagedApplyEvent

type ManagedApplyReporter = update.ManagedApplyReporter

type ManagedApplyReporterFunc = update.ManagedApplyReporterFunc

type ManagedApplyReporterFactory func(index, total int, update ManagedUpdateView) ManagedApplyReporter

type UpdateApplyBatchRequest struct {
	Pending     []ManagedUpdateHandle
	ReporterFor ManagedApplyReporterFactory
}

type UpdateSourceRequest struct {
	ID     string
	Source *UpdateSourceInput
}

type AddAction string

const (
	AddActionIntegrate         AddAction = "integrate"
	AddActionReintegrate       AddAction = "reintegrate"
	AddActionInstall           AddAction = "install"
	AddActionAlreadyIntegrated AddAction = "already_integrated"
)

type AddResult struct {
	Action AddAction
	Status string

	App     *AppDetails
	Plan    *DryRunPlan
	Package *PackageView

	AlreadyIntegrated bool
}

type ListResult struct {
	Apps            []*AppDetails
	TotalCount      int
	IntegratedCount int
	UnlinkedCount   int
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

type UpgradeResult struct {
	CurrentVersion   string
	LatestVersion    string
	InstalledVersion string
	UpToDate         bool
	Upgraded         bool
}

type UpdateResult struct {
	Mode          UpdateMode
	Rows          []ManagedUpdateCheckRow
	Pending       []ManagedUpdateView
	CheckFailures int
	Failures      []ManagedCheckFailureView
	Applied       []ManagedApplyResultView
	ApplySkipped  bool
	ApplyFailures int

	Source           *UpdateSourceResult
	Plan             *DryRunPlan
	SourceChange     *UpdateSourceChangeView
	NoEmbeddedSource bool
	SourceUnchanged  bool
}

type ManagedUpdateCheckResult struct {
	Rows           []ManagedUpdateCheckRow
	Pending        []ManagedUpdateView
	PendingHandles []ManagedUpdateHandle `json:"-"`
	CheckFailures  int
	Failures       []ManagedCheckFailureView
}

type ManagedUpdateCheckRow struct {
	App       *AppSummary        `json:"app,omitempty"`
	Update    *ManagedUpdateView `json:"update,omitempty"`
	Status    string             `json:"status"`
	CheckedAt string             `json:"checked_at,omitempty"`
	Error     string             `json:"error,omitempty"`
}

type ManagedCheckFailureView struct {
	AppID  string `json:"app_id,omitempty"`
	Reason string `json:"reason"`
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
	ConfirmUpdateSourceReplace(existing, incoming *UpdateSourceView) (bool, error)
}

type PackageViewAmbiguityResolver interface {
	ResolvePackageViewAmbiguity(metadata *PackageView) (*PackageView, error)
}
