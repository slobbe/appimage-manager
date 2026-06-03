package services

import (
	"context"

	"github.com/slobbe/appimage-manager/internal/app/selfupdate"
	"github.com/slobbe/appimage-manager/internal/app/update"
	"github.com/slobbe/appimage-manager/internal/domain"
)

type App = domain.App

type PackageRef = domain.PackageRef

type PackageMetadata = domain.PackageMetadata

type AssetCandidate = domain.AssetCandidate

type UpdateKind = domain.UpdateKind

type UpdateSource = domain.UpdateSource

type ZsyncUpdateSource = domain.ZsyncUpdateSource

type GitHubReleaseUpdateSource = domain.GitHubReleaseUpdateSource

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

type AimSelfUpdateCheckResult = selfupdate.AimSelfUpdateCheckResult

type InstallerSelfUpdateResult = selfupdate.InstallerSelfUpdateResult

type SelfUpdateService interface {
	SelfUpdate(ctx context.Context, req SelfUpdateRequest) (*SelfUpdateResult, error)
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

type ManagedAppCompletion struct {
	ID   string
	Name string
}

type AddRequest struct {
	Target AddTargetInput

	DryRun bool

	SHA256       string
	AssetPattern string

	ConfirmUpdateSourceReplace UpdateSourceReplaceConfirmer
	ResolvePackageAmbiguity    PackageMetadataAmbiguityResolver
}

type AddTargetInput struct {
	Positional string
	URL        string
	Provider   *domain.PackageRef
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
	App       *domain.App
	LocalPath string
}

type InstallDirectURLRequest struct {
	URL    string
	SHA256 string
}

type InstallPackageRefRequest struct {
	Ref                  domain.PackageRef
	AssetPattern         string
	ResolveViewAmbiguity PackageMetadataAmbiguityResolver
}

type PackageRefInfoRequest struct {
	Ref          domain.PackageRef
	AssetPattern string
}

type InfoRequest struct {
	Input        string
	Provider     *domain.PackageRef
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

type SelfUpdateRequest struct {
	CurrentVersion string
	DryRun         bool
	PreRelease     bool
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

	Source            *domain.UpdateSource
	UseEmbeddedSource bool

	OnCacheHit           func(appID string)
	ConfirmApply         UpdateApplyConfirmer
	ConfirmSourceReplace UpdateSourceReplaceConfirmer
	ConfirmSourceUnset   UpdateSourceUnsetConfirmer
	ReporterFor          ManagedApplyReporterFactory
}

type UpdateApplyConfirmation struct {
	TargetID string
	Pending  []ManagedUpdate
}

type UpdateApplyConfirmer interface {
	ConfirmUpdateApply(UpdateApplyConfirmation) (bool, error)
}

type UpdateSourceUnsetConfirmer interface {
	ConfirmUpdateSourceUnset(current *domain.UpdateSource) (bool, error)
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

type ManagedUpdate = update.ManagedUpdate

type ManagedApplyResult = update.ManagedApplyResult

type ManagedApplyEvent = update.ManagedApplyEvent

type ManagedApplyReporter = update.ManagedApplyReporter

type ManagedApplyReporterFunc = update.ManagedApplyReporterFunc

type ManagedApplyReporterFactory func(index, total int, update ManagedUpdate) ManagedApplyReporter

type UpdateApplyBatchRequest struct {
	Pending     []ManagedUpdateHandle
	ReporterFor ManagedApplyReporterFactory
}

type updateSourceRequest struct {
	ID     string
	Source *domain.UpdateSource
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

	App     *domain.App
	Plan    *DryRunPlan
	Package *domain.PackageMetadata

	AlreadyIntegrated bool
}

type ListResult struct {
	Apps            []*domain.App
	TotalCount      int
	IntegratedCount int
	UnlinkedCount   int
}

type InfoResult struct {
	Kind            InfoKind
	App             *domain.App
	AppImageInfo    *AppImageInfoView
	PackageMetadata *domain.PackageMetadata
	EmbeddedUpdate  *domain.UpdateSource
}

type RemoveResult struct {
	Action string
	App    *domain.App
	Unlink bool
	Paths  []string
}

type SelfUpdateResult struct {
	CurrentVersion   string
	LatestVersion    string
	InstalledVersion string
	UpToDate         bool
	CurrentAhead     bool
	PreRelease       bool
	Updated          bool
}

type UpdateResult struct {
	Mode          UpdateMode
	Rows          []ManagedUpdateCheckRow
	Pending       []ManagedUpdate
	CheckFailures int
	Failures      []ManagedCheckFailureView
	Applied       []ManagedApplyResult
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
	Pending        []ManagedUpdate
	PendingHandles []ManagedUpdateHandle `json:"-"`
	CheckFailures  int
	Failures       []ManagedCheckFailureView
}

type ManagedUpdateCheckRow struct {
	App       *domain.App    `json:"app,omitempty"`
	Update    *ManagedUpdate `json:"update,omitempty"`
	Status    string         `json:"status"`
	CheckedAt string         `json:"checked_at,omitempty"`
	Error     string         `json:"error,omitempty"`
}

type ManagedCheckFailureView struct {
	AppID  string `json:"app_id,omitempty"`
	Reason string `json:"reason"`
}

type UpdateApplyBatchResult struct {
	Results []ManagedApplyResult
}

type UpdateSourceResult struct {
	ID      string
	Source  *domain.UpdateSource
	Changed bool
}

type DryRunPlan struct {
	Action string
	Target string
	Values map[string]interface{}

	TargetKind         string
	App                *domain.App
	Package            *domain.PackageMetadata
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

type PackageMetadataAmbiguityResolver interface {
	ResolvePackageMetadataAmbiguity(metadata *domain.PackageMetadata) (*domain.PackageMetadata, error)
}
