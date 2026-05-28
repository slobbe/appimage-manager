package services

import (
	"strings"

	appimageapp "github.com/slobbe/appimage-manager/internal/app/appimage"
	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	"github.com/slobbe/appimage-manager/internal/domain"
)

// ProviderRef is a domain-free provider package reference for app-service callers.
type ProviderRef struct {
	Provider string `json:"provider"`
	Ref      string `json:"ref"`
}

// AppSummary is the compact app view intended for list-like app-service results.
type AppSummary struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	Integrated      bool   `json:"integrated"`
	UpdateAvailable bool   `json:"update_available"`
	LatestVersion   string `json:"latest_version,omitempty"`
	LastCheckedAt   string `json:"last_checked_at,omitempty"`
}

// AppDetails is the detailed app view intended for command results and info output.
type AppDetails struct {
	AppSummary
	ReplacesID       string            `json:"-"`
	ExecPath         string            `json:"exec_path,omitempty"`
	IconPath         string            `json:"icon_path,omitempty"`
	DesktopEntryPath string            `json:"desktop_entry_path,omitempty"`
	DesktopEntryLink string            `json:"desktop_entry_link,omitempty"`
	AddedAt          string            `json:"added_at,omitempty"`
	UpdatedAt        string            `json:"updated_at,omitempty"`
	SHA256           string            `json:"sha256,omitempty"`
	SHA1             string            `json:"sha1,omitempty"`
	Source           *SourceView       `json:"source,omitempty"`
	UpdateSource     *UpdateSourceView `json:"update,omitempty"`
}

// SourceView describes where a managed app came from without exposing domain types.
type SourceView struct {
	Kind          string                   `json:"kind"`
	LocalFile     *LocalFileSourceView     `json:"local_file,omitempty"`
	DirectURL     *DirectURLSourceView     `json:"direct_url,omitempty"`
	GitHubRelease *GitHubReleaseSourceView `json:"github_release,omitempty"`
}

type LocalFileSourceView struct {
	IntegratedAt string `json:"integrated_at,omitempty"`
	OriginalPath string `json:"original_path,omitempty"`
}

type DirectURLSourceView struct {
	URL          string `json:"url"`
	SHA256       string `json:"sha256,omitempty"`
	DownloadedAt string `json:"downloaded_at,omitempty"`
}

type GitHubReleaseSourceView struct {
	Repo         string `json:"repo"`
	Asset        string `json:"asset"`
	Tag          string `json:"tag,omitempty"`
	AssetName    string `json:"asset_name,omitempty"`
	DownloadedAt string `json:"downloaded_at,omitempty"`
}

// UpdateSourceView is the domain-free representation of a managed app update source.
type UpdateSourceView struct {
	Kind          string                   `json:"kind"`
	Zsync         *ZsyncUpdateSourceView   `json:"zsync,omitempty"`
	GitHubRelease *GitHubReleaseUpdateView `json:"github_release,omitempty"`
}

type ZsyncUpdateSourceView struct {
	UpdateInfo string `json:"update_info"`
	Transport  string `json:"transport,omitempty"`
}

type GitHubReleaseUpdateView struct {
	Repo        string `json:"repo"`
	Asset       string `json:"asset"`
	ReleaseKind string `json:"release_kind,omitempty"`
}

// UpdateSourceChangeView describes a planned or completed update-source change.
type UpdateSourceChangeView struct {
	ID       string            `json:"id"`
	Current  *UpdateSourceView `json:"current_source,omitempty"`
	Incoming *UpdateSourceView `json:"incoming_source,omitempty"`
	Changed  bool              `json:"changed,omitempty"`
}

// PackageView is the domain-free package metadata view for discovery/info/install flows.
type PackageView struct {
	Name            string               `json:"name"`
	Provider        string               `json:"provider"`
	Ref             ProviderRef          `json:"ref"`
	RepoURL         string               `json:"repo_url,omitempty"`
	LatestVersion   string               `json:"latest_version,omitempty"`
	AssetName       string               `json:"asset_name,omitempty"`
	AssetPattern    string               `json:"asset_pattern,omitempty"`
	DownloadURL     string               `json:"download_url,omitempty"`
	AssetCandidates []AssetCandidateView `json:"asset_candidates,omitempty"`
	AssetAmbiguous  bool                 `json:"asset_ambiguous,omitempty"`
	AssetReason     string               `json:"asset_reason,omitempty"`
	Installable     bool                 `json:"installable"`
	InstallReason   string               `json:"install_reason,omitempty"`
	ReleaseTag      string               `json:"release_tag,omitempty"`
	Summary         string               `json:"summary,omitempty"`
}

type AssetCandidateView struct {
	Name        string `json:"name"`
	DownloadURL string `json:"download_url"`
	Arch        string `json:"arch,omitempty"`
	ArchLabel   string `json:"arch_label,omitempty"`
}

// AppImageInfoView is the app-service view of AppImage metadata.
type AppImageInfoView struct {
	Name        string `json:"name"`
	ID          string `json:"id"`
	DesktopStem string `json:"desktop_stem,omitempty"`
	Version     string `json:"version"`
}

// ManagedUpdateView describes a pending managed update without exposing lower app types.
type ManagedUpdateView struct {
	App            *AppSummary `json:"app,omitempty"`
	URL            string      `json:"url,omitempty"`
	Asset          string      `json:"asset,omitempty"`
	Label          string      `json:"label,omitempty"`
	Available      bool        `json:"available"`
	Latest         string      `json:"latest,omitempty"`
	ExpectedSHA1   string      `json:"expected_sha1,omitempty"`
	ExpectedSHA256 string      `json:"expected_sha256,omitempty"`
	Transport      string      `json:"transport,omitempty"`
	ZsyncURL       string      `json:"zsync_url,omitempty"`
	FromKind       string      `json:"from_kind,omitempty"`
}

// ManagedApplyResultView describes the result of applying one managed update.
type ManagedApplyResultView struct {
	Index      int         `json:"index"`
	App        *AppSummary `json:"app,omitempty"`
	UpdatedApp *AppDetails `json:"updated_app,omitempty"`
	Error      string      `json:"error,omitempty"`
}

func providerRefFromDomain(ref domain.PackageRef) ProviderRef {
	return ProviderRef{Provider: strings.TrimSpace(ref.Kind), Ref: strings.TrimSpace(ref.ProviderRef)}
}

func appSummaryFromDomain(app *domain.App) *AppSummary {
	if app == nil {
		return nil
	}
	return &AppSummary{
		ID:              strings.TrimSpace(app.ID),
		Name:            strings.TrimSpace(app.Name),
		Version:         strings.TrimSpace(app.Version),
		Integrated:      strings.TrimSpace(app.DesktopEntryLink) != "",
		UpdateAvailable: app.UpdateAvailable,
		LatestVersion:   strings.TrimSpace(app.LatestVersion),
		LastCheckedAt:   strings.TrimSpace(app.LastCheckedAt),
	}
}

func appDetailsFromDomain(app *domain.App) *AppDetails {
	if app == nil {
		return nil
	}
	summary := appSummaryFromDomain(app)
	details := &AppDetails{
		ReplacesID:       strings.TrimSpace(app.ReplacesID),
		ExecPath:         strings.TrimSpace(app.ExecPath),
		IconPath:         strings.TrimSpace(app.IconPath),
		DesktopEntryPath: strings.TrimSpace(app.DesktopEntryPath),
		DesktopEntryLink: strings.TrimSpace(app.DesktopEntryLink),
		AddedAt:          strings.TrimSpace(app.AddedAt),
		UpdatedAt:        strings.TrimSpace(app.UpdatedAt),
		SHA256:           strings.TrimSpace(app.SHA256),
		SHA1:             strings.TrimSpace(app.SHA1),
		Source:           sourceViewFromDomain(app.Source),
		UpdateSource:     updateSourceViewFromDomain(app.Update),
	}
	if summary != nil {
		details.AppSummary = *summary
	}
	return details
}

func sourceViewFromDomain(source domain.Source) *SourceView {
	kind := strings.TrimSpace(string(source.Kind))
	if kind == "" {
		return nil
	}
	view := &SourceView{Kind: kind}
	if source.LocalFile != nil {
		view.LocalFile = &LocalFileSourceView{
			IntegratedAt: strings.TrimSpace(source.LocalFile.IntegratedAt),
			OriginalPath: strings.TrimSpace(source.LocalFile.OriginalPath),
		}
	}
	if source.DirectURL != nil {
		view.DirectURL = &DirectURLSourceView{
			URL:          strings.TrimSpace(source.DirectURL.URL),
			SHA256:       strings.TrimSpace(source.DirectURL.SHA256),
			DownloadedAt: strings.TrimSpace(source.DirectURL.DownloadedAt),
		}
	}
	if source.GitHubRelease != nil {
		view.GitHubRelease = &GitHubReleaseSourceView{
			Repo:         strings.TrimSpace(source.GitHubRelease.Repo),
			Asset:        strings.TrimSpace(source.GitHubRelease.Asset),
			Tag:          strings.TrimSpace(source.GitHubRelease.Tag),
			AssetName:    strings.TrimSpace(source.GitHubRelease.AssetName),
			DownloadedAt: strings.TrimSpace(source.GitHubRelease.DownloadedAt),
		}
	}
	return view
}

func UpdateSourceViewFromDomain(source *domain.UpdateSource) *UpdateSourceView {
	return updateSourceViewFromDomain(source)
}

func updateSourceViewFromDomain(source *domain.UpdateSource) *UpdateSourceView {
	if source == nil {
		return nil
	}
	view := &UpdateSourceView{Kind: strings.TrimSpace(string(source.Kind))}
	if source.Zsync != nil {
		view.Zsync = &ZsyncUpdateSourceView{
			UpdateInfo: strings.TrimSpace(source.Zsync.UpdateInfo),
			Transport:  strings.TrimSpace(source.Zsync.Transport),
		}
	}
	if source.GitHubRelease != nil {
		view.GitHubRelease = &GitHubReleaseUpdateView{
			Repo:        strings.TrimSpace(source.GitHubRelease.Repo),
			Asset:       strings.TrimSpace(source.GitHubRelease.Asset),
			ReleaseKind: strings.TrimSpace(source.GitHubRelease.ReleaseKind),
		}
	}
	return view
}

func packageViewFromDomain(metadata *domain.PackageMetadata) *PackageView {
	if metadata == nil {
		return nil
	}
	candidates := make([]AssetCandidateView, 0, len(metadata.AssetCandidates))
	for _, candidate := range metadata.AssetCandidates {
		candidates = append(candidates, AssetCandidateView{
			Name:        strings.TrimSpace(candidate.Name),
			DownloadURL: strings.TrimSpace(candidate.DownloadURL),
			Arch:        strings.TrimSpace(candidate.Arch),
			ArchLabel:   strings.TrimSpace(candidate.ArchLabel),
		})
	}
	return &PackageView{
		Name:            strings.TrimSpace(metadata.Name),
		Provider:        strings.TrimSpace(metadata.Provider),
		Ref:             providerRefFromDomain(metadata.Ref),
		RepoURL:         strings.TrimSpace(metadata.RepoURL),
		LatestVersion:   strings.TrimSpace(metadata.LatestVersion),
		AssetName:       strings.TrimSpace(metadata.AssetName),
		AssetPattern:    strings.TrimSpace(metadata.AssetPattern),
		DownloadURL:     strings.TrimSpace(metadata.DownloadURL),
		AssetCandidates: candidates,
		AssetAmbiguous:  metadata.AssetAmbiguous,
		AssetReason:     strings.TrimSpace(metadata.AssetReason),
		Installable:     metadata.Installable,
		InstallReason:   strings.TrimSpace(metadata.InstallReason),
		ReleaseTag:      strings.TrimSpace(metadata.ReleaseTag),
		Summary:         strings.TrimSpace(metadata.Summary),
	}
}

func appImageInfoViewFromAppImageInfo(info *appimageapp.AppInfo) *AppImageInfoView {
	if info == nil {
		return nil
	}
	return &AppImageInfoView{
		Name:        strings.TrimSpace(info.Name),
		ID:          strings.TrimSpace(info.ID),
		DesktopStem: strings.TrimSpace(info.DesktopStem),
		Version:     strings.TrimSpace(info.Version),
	}
}

func managedUpdateViewFromAppUpdate(update *appupdate.ManagedUpdate) *ManagedUpdateView {
	if update == nil {
		return nil
	}
	return &ManagedUpdateView{
		App:            appSummaryFromDomain(update.App),
		URL:            strings.TrimSpace(update.URL),
		Asset:          strings.TrimSpace(update.Asset),
		Label:          strings.TrimSpace(update.Label),
		Available:      update.Available,
		Latest:         strings.TrimSpace(update.Latest),
		ExpectedSHA1:   strings.TrimSpace(update.ExpectedSHA1),
		ExpectedSHA256: strings.TrimSpace(update.ExpectedSHA256),
		Transport:      strings.TrimSpace(update.Transport),
		ZsyncURL:       strings.TrimSpace(update.ZsyncURL),
		FromKind:       strings.TrimSpace(string(update.FromKind)),
	}
}

func managedApplyResultViewFromAppUpdate(result appupdate.ManagedApplyResult) ManagedApplyResultView {
	view := ManagedApplyResultView{
		Index:      result.Index,
		App:        appSummaryFromDomain(result.App),
		UpdatedApp: appDetailsFromDomain(result.UpdatedApp),
	}
	if result.Error != nil {
		view.Error = result.Error.Error()
	}
	return view
}
