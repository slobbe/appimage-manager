package app

import (
	"context"

	"aim/internal/domain"
)

type Service interface {
	Add(ctx context.Context, req AddRequest) (AddResult, error)
	Remove(ctx context.Context, req RemoveRequest) error
	Update(ctx context.Context, req UpdateRequest) (UpdateResult, error)
	SetUpdateSource(ctx context.Context, req SetUpdateSourceRequest) (SetUpdateSourceResult, error)
	UnsetUpdateSource(ctx context.Context, req UnsetUpdateSourceRequest) error
	List(ctx context.Context, req ListRequest) (ListResult, error)
	Info(ctx context.Context, req InfoRequest) (InfoResult, error)
	SelfUpdate(ctx context.Context, req SelfUpdateRequest) (SelfUpdateResult, error)
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
