package app

import (
	"context"

	"github.com/slobbe/appimage-manager/internal/domain"
)

type Service interface {
	Adder
	Remover
	Updater
	IDManager
	Lister
	Informer
	SelfUpdateRunner
	PathProvider
}

type Adder interface {
	Add(ctx context.Context, req AddRequest) (AddResult, error)
}

type Remover interface {
	Remove(ctx context.Context, req RemoveRequest) error
}

type Updater interface {
	Update(ctx context.Context, req UpdateRequest) (UpdateResult, error)
	SetUpdateSource(ctx context.Context, req SetUpdateSourceRequest) (SetUpdateSourceResult, error)
	UnsetUpdateSource(ctx context.Context, req UnsetUpdateSourceRequest) error
}

type IDManager interface {
	SetID(ctx context.Context, req SetIDRequest) (SetIDResult, error)
}

type Lister interface {
	List(ctx context.Context, req ListRequest) (ListResult, error)
}

type Informer interface {
	Info(ctx context.Context, req InfoRequest) (InfoResult, error)
}

type SelfUpdateRunner interface {
	SelfUpdate(ctx context.Context, req SelfUpdateRequest) (SelfUpdateResult, error)
}

type PathProvider interface {
	Paths(ctx context.Context, req PathsRequest) (PathsResult, error)
}

type AddRequest struct {
	Path         string
	GitHubRepo   string
	AssetPattern string
	Prerelease   bool
	Activity     ActivityReporter
}

type AddResult struct {
	App domain.App
}

type RemoveRequest struct {
	Name     string
	Activity ActivityReporter
}

type UpdateRequest struct {
	Target       string
	CheckOnly    bool
	Activity     ActivityReporter
	Confirmation UpdateConfirmation
}

type UpdateConfirmation interface {
	ConfirmUpdates(ctx context.Context, updates []UpdateCandidate) (bool, error)
}

type UpdateCandidate struct {
	ID             string
	CurrentVersion string
	NewVersion     string
}

type UpdateResult struct {
	Applied bool
	Updates []UpdateCandidate
}

type SetUpdateSourceRequest struct {
	ID           string
	GitHubRepo   string
	AssetPattern string
	Prerelease   bool
	Embedded     bool
}

type SetUpdateSourceResult struct {
	ID           string
	UpdateSource domain.UpdateSource
}

type UnsetUpdateSourceRequest struct {
	ID string
}

type SetIDRequest struct {
	CurrentID string
	NewID     string
	Auto      bool
	Activity  ActivityReporter
}

type SetIDResult struct {
	PreviousID string
	ID         string
	App        domain.App
	Changed    bool
}

type ListRequest struct{}

type ListResult struct {
	Items []ListItem
}

type ListItem struct {
	ID      string
	Name    string
	Version string
}

type InfoRequest struct {
	Target string
}

type InfoResult struct {
	ID           string
	Name         string
	Version      string
	ExecPath     string
	Installed    bool
	TargetKind   string
	Source       domain.Source
	UpdateSource domain.UpdateSource
}

type SelfUpdateRequest struct {
	Prerelease   bool
	Activity     ActivityReporter
	Confirmation SelfUpdateConfirmation
}

type SelfUpdateConfirmation interface {
	ConfirmSelfUpdate(ctx context.Context, update SelfUpdateCandidate) (bool, error)
}

type SelfUpdateCandidate struct {
	CurrentVersion string
	NewVersion     string
	AssetName      string
	AssetSizeBytes int64
}

type SelfUpdateResult struct {
	Applied bool
	Update  SelfUpdateCandidate
}

type PathsRequest struct{}

type PathsResult struct {
	ConfigFile  string
	AppImageDir string
	CacheDir    string
	DesktopDir  string
	IconDir     string
}
