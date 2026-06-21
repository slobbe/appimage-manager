package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/domain"
)

// Repository persists integrated apps in a JSON file.
type Repository struct {
	Path string
}

// NewRepository creates a JSON app repository backed by path.
func NewRepository(path string) Repository {
	return Repository{Path: path}
}

var _ app.AppRepository = Repository{}

const currentSchemaVersion = 2

var repositoryMu sync.Mutex

type databaseFile struct {
	SchemaVersion int         `json:"schema_version"`
	Apps          []appRecord `json:"apps"`
}

type appRecord struct {
	ID               string              `json:"id"`
	Name             string              `json:"name"`
	Version          string              `json:"version,omitempty"`
	AppImagePath     string              `json:"app_image_path"`
	DesktopEntryPath string              `json:"desktop_entry_path,omitempty"`
	IconPath         string              `json:"icon_path,omitempty"`
	Source           *sourceRecord       `json:"source,omitempty"`
	UpdateSource     *updateSourceRecord `json:"update_source,omitempty"`
}

type sourceRecord struct {
	Kind          string                     `json:"kind"`
	LocalFile     *localFileSourceRecord     `json:"local_file,omitempty"`
	GitHubRelease *githubReleaseSourceRecord `json:"github_release,omitempty"`
}

type updateSourceRecord struct {
	Embedded          bool   `json:"embedded"`
	Kind              string `json:"kind"`
	Raw               string `json:"raw,omitempty"`
	Transport         string `json:"transport,omitempty"`
	Repo              string `json:"repo,omitempty"`
	Path              string `json:"path,omitempty"`
	Prerelease        bool   `json:"prerelease,omitempty"`
	ReleaseTag        string `json:"release_tag,omitempty"`
	AssetPattern      string `json:"asset_pattern,omitempty"`
	ZsyncAssetPattern string `json:"zsync_asset_pattern,omitempty"`
	URL               string `json:"url,omitempty"`
}

type localFileSourceRecord struct {
	Path         string `json:"path"`
	IntegratedAt string `json:"integrated_at,omitempty"`
}

type githubReleaseSourceRecord struct {
	Repo         string `json:"repo"`
	Tag          string `json:"tag,omitempty"`
	Asset        string `json:"asset,omitempty"`
	DownloadURL  string `json:"download_url,omitempty"`
	SizeBytes    int64  `json:"size_bytes,omitempty"`
	DownloadedAt string `json:"downloaded_at,omitempty"`
}

// Save inserts or replaces an app by ID.
func (r Repository) Save(ctx context.Context, domainApp domain.App) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.validate(); err != nil {
		return err
	}
	if strings.TrimSpace(domainApp.ID) == "" {
		return fmt.Errorf("save app: app id is required")
	}

	unlock, err := r.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	db, err := r.load(ctx)
	if err != nil {
		return err
	}

	record := recordFromDomainApp(domainApp)
	replaced := false
	for i, existing := range db.Apps {
		if existing.ID == record.ID {
			db.Apps[i] = record
			replaced = true
			break
		}
	}
	if !replaced {
		db.Apps = append(db.Apps, record)
	}
	sortAppRecords(db.Apps)

	return r.save(ctx, db)
}

// Find returns an app by ID.
func (r Repository) Find(ctx context.Context, id string) (domain.App, error) {
	if err := ctx.Err(); err != nil {
		return domain.App{}, err
	}
	if err := r.validate(); err != nil {
		return domain.App{}, err
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return domain.App{}, fmt.Errorf("find app: app id is required")
	}

	db, err := r.load(ctx)
	if err != nil {
		return domain.App{}, err
	}

	for _, record := range db.Apps {
		if record.ID == id {
			return record.toDomainApp()
		}
	}

	return domain.App{}, app.ErrAppNotFound
}

// List returns all apps sorted by ID.
func (r Repository) List(ctx context.Context) ([]domain.App, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.validate(); err != nil {
		return nil, err
	}

	db, err := r.load(ctx)
	if err != nil {
		return nil, err
	}
	sortAppRecords(db.Apps)

	result := make([]domain.App, 0, len(db.Apps))
	for _, record := range db.Apps {
		domainApp, err := record.toDomainApp()
		if err != nil {
			return nil, err
		}
		result = append(result, domainApp)
	}

	return result, nil
}

// Delete removes an app by ID.
func (r Repository) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.validate(); err != nil {
		return err
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("delete app: app id is required")
	}

	unlock, err := r.lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	db, err := r.load(ctx)
	if err != nil {
		return err
	}

	for i, record := range db.Apps {
		if record.ID == id {
			db.Apps = append(db.Apps[:i], db.Apps[i+1:]...)
			return r.save(ctx, db)
		}
	}

	return app.ErrAppNotFound
}

func (r Repository) validate() error {
	if strings.TrimSpace(r.Path) == "" {
		return fmt.Errorf("storage path is required")
	}

	return nil
}

func (r Repository) lock(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	repositoryMu.Lock()
	if err := ctx.Err(); err != nil {
		repositoryMu.Unlock()
		return nil, err
	}

	return repositoryMu.Unlock, nil
}

func (r Repository) load(ctx context.Context) (databaseFile, error) {
	if err := ctx.Err(); err != nil {
		return databaseFile{}, err
	}

	bytes, err := os.ReadFile(r.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return databaseFile{}, nil
		}
		return databaseFile{}, fmt.Errorf("read app database %q: %w", r.Path, err)
	}
	if len(bytes) == 0 {
		return databaseFile{}, nil
	}

	var db databaseFile
	if err := json.Unmarshal(bytes, &db); err != nil {
		return databaseFile{}, fmt.Errorf("parse app database %q: %w", r.Path, err)
	}

	return db, nil
}

func (r Repository) save(ctx context.Context, db databaseFile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	db.SchemaVersion = currentSchemaVersion
	if err := os.MkdirAll(filepath.Dir(r.Path), 0o755); err != nil {
		return fmt.Errorf("create app database directory %q: %w", filepath.Dir(r.Path), err)
	}

	bytes, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("encode app database: %w", err)
	}
	bytes = append(bytes, '\n')

	temporaryFile, err := os.CreateTemp(filepath.Dir(r.Path), filepath.Base(r.Path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create app database temporary file in %q: %w", filepath.Dir(r.Path), err)
	}
	temporaryPath := temporaryFile.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	if _, err := temporaryFile.Write(bytes); err != nil {
		_ = temporaryFile.Close()
		return fmt.Errorf("write app database %q: %w", temporaryPath, err)
	}
	if err := temporaryFile.Chmod(0o644); err != nil {
		_ = temporaryFile.Close()
		return fmt.Errorf("set app database permissions %q: %w", temporaryPath, err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close app database %q: %w", temporaryPath, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, r.Path); err != nil {
		return fmt.Errorf("replace app database %q: %w", r.Path, err)
	}
	removeTemporary = false

	return nil
}

func recordFromDomainApp(domainApp domain.App) appRecord {
	return appRecord{
		ID:               domainApp.ID,
		Name:             domainApp.Name,
		Version:          domainApp.Version.String(),
		AppImagePath:     domainApp.AppImagePath,
		DesktopEntryPath: domainApp.DesktopEntryPath,
		IconPath:         domainApp.IconPath,
		Source:           recordFromDomainSource(domainApp.Source),
		UpdateSource:     recordFromDomainUpdateSource(domainApp.UpdateSource),
	}
}

func recordFromDomainUpdateSource(source domain.UpdateSource) *updateSourceRecord {
	if source.Kind == domain.UpdateSourceKindUnknown && !source.Embedded {
		return nil
	}
	switch source.Kind {
	case domain.UpdateSourceKindGitHub, domain.UpdateSourceKindLocalFile, domain.UpdateSourceKindZsync, domain.UpdateSourceKindUnsupported:
		return &updateSourceRecord{
			Embedded:          source.Embedded,
			Kind:              string(source.Kind),
			Raw:               source.Raw,
			Transport:         source.Transport,
			Repo:              source.Repo,
			Path:              source.Path,
			Prerelease:        source.Prerelease,
			ReleaseTag:        source.ReleaseTag,
			AssetPattern:      source.AssetPattern,
			ZsyncAssetPattern: source.ZsyncAssetPattern,
			URL:               source.URL,
		}
	default:
		return nil
	}
}

func (r *updateSourceRecord) toDomainUpdateSource() domain.UpdateSource {
	if r == nil {
		return domain.UpdateSource{}
	}
	return domain.UpdateSource{
		Embedded:          r.Embedded,
		Kind:              domain.UpdateSourceKind(r.Kind),
		Raw:               strings.TrimSpace(r.Raw),
		Transport:         strings.TrimSpace(r.Transport),
		Repo:              strings.TrimSpace(r.Repo),
		Path:              strings.TrimSpace(r.Path),
		Prerelease:        r.Prerelease,
		ReleaseTag:        strings.TrimSpace(r.ReleaseTag),
		AssetPattern:      strings.TrimSpace(r.AssetPattern),
		ZsyncAssetPattern: strings.TrimSpace(r.ZsyncAssetPattern),
		URL:               strings.TrimSpace(r.URL),
	}
}

func recordFromDomainSource(source domain.Source) *sourceRecord {
	switch source.Kind {
	case domain.SourceKindLocal:
		return &sourceRecord{
			Kind: string(domain.SourceKindLocal),
			LocalFile: &localFileSourceRecord{
				Path:         source.LocalFile.Path,
				IntegratedAt: formatSourceTime(source.LocalFile.IntegratedAt),
			},
		}
	case domain.SourceKindGitHub:
		return &sourceRecord{
			Kind: string(domain.SourceKindGitHub),
			GitHubRelease: &githubReleaseSourceRecord{
				Repo:         source.GitHubRelease.Repo,
				Tag:          source.GitHubRelease.Tag,
				Asset:        source.GitHubRelease.Asset,
				DownloadURL:  source.GitHubRelease.DownloadURL,
				SizeBytes:    source.GitHubRelease.SizeBytes,
				DownloadedAt: formatSourceTime(source.GitHubRelease.DownloadedAt),
			},
		}
	default:
		return nil
	}
}

func (r *sourceRecord) toDomainSource() domain.Source {
	if r == nil {
		return domain.Source{}
	}

	switch domain.SourceKind(r.Kind) {
	case domain.SourceKindLocal:
		if r.LocalFile == nil {
			return domain.Source{Kind: domain.SourceKindLocal}
		}
		return domain.NewLocalSource(r.LocalFile.Path, parseSourceTime(r.LocalFile.IntegratedAt))
	case domain.SourceKindGitHub:
		if r.GitHubRelease == nil {
			return domain.Source{Kind: domain.SourceKindGitHub}
		}
		return domain.NewGitHubReleaseSource(
			r.GitHubRelease.Repo,
			r.GitHubRelease.Tag,
			r.GitHubRelease.Asset,
			r.GitHubRelease.DownloadURL,
			r.GitHubRelease.SizeBytes,
			parseSourceTime(r.GitHubRelease.DownloadedAt),
		)
	default:
		return domain.Source{}
	}
}

func formatSourceTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}

	return value.UTC().Format(time.RFC3339)
}

func parseSourceTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}

	return parsed.UTC()
}

func (r appRecord) toDomainApp() (domain.App, error) {
	var version domain.Version
	if r.Version != "" {
		parsed, ok := domain.ParseVersion(r.Version)
		if !ok {
			return domain.App{}, fmt.Errorf("parse stored version for app %q: %q", r.ID, r.Version)
		}
		version = parsed
	}

	return domain.App{
		ID:               r.ID,
		Name:             r.Name,
		Version:          version,
		AppImagePath:     r.AppImagePath,
		DesktopEntryPath: r.DesktopEntryPath,
		IconPath:         r.IconPath,
		Source:           r.Source.toDomainSource(),
		UpdateSource:     r.UpdateSource.toDomainUpdateSource(),
	}, nil
}

func sortAppRecords(records []appRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		return records[i].ID < records[j].ID
	})
}
