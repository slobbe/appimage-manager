package migration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const currentSchemaVersion = 2

type V1Options struct {
	SourcePath  string
	DestPath    string
	AppImageDir string
	DesktopDir  string
	Force       bool
}

func MigrateV1(ctx context.Context, opts V1Options) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if strings.TrimSpace(opts.SourcePath) == "" {
		return false, errors.New("legacy database source path is required")
	}
	if strings.TrimSpace(opts.DestPath) == "" {
		return false, errors.New("database destination path is required")
	}
	if strings.TrimSpace(opts.AppImageDir) == "" {
		return false, errors.New("appimage directory is required")
	}
	if strings.TrimSpace(opts.DesktopDir) == "" {
		return false, errors.New("desktop directory is required")
	}

	if _, err := os.Stat(opts.SourcePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat legacy database %q: %w", opts.SourcePath, err)
	}
	if !opts.Force {
		if _, err := os.Stat(opts.DestPath); err == nil {
			return false, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("stat database %q: %w", opts.DestPath, err)
		}
	}

	legacy, err := readLegacyDatabase(opts.SourcePath)
	if err != nil {
		return false, err
	}
	if legacy.SchemaVersion != 1 {
		return false, fmt.Errorf("legacy database schemaVersion = %d, want 1", legacy.SchemaVersion)
	}

	plans := migrationPlans(legacy, opts.AppImageDir, opts.DesktopDir)
	v2 := databaseFile{SchemaVersion: currentSchemaVersion, Apps: make([]appRecord, 0, len(plans))}
	for _, plan := range plans {
		v2.Apps = append(v2.Apps, plan.record)
	}

	if err := preflightMigration(opts, plans); err != nil {
		return false, err
	}

	rollback := migrationRollback{}
	for _, plan := range plans {
		if err := ctx.Err(); err != nil {
			return false, rollback.rollback(err)
		}
		moved, err := moveIfNeeded(plan.oldAppImagePath, plan.newAppImagePath)
		if err != nil {
			return false, rollback.rollback(err)
		}
		if moved {
			oldPath := plan.oldAppImagePath
			newPath := plan.newAppImagePath
			rollback.add(func() error {
				if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
					return fmt.Errorf("create rollback appimage directory %q: %w", filepath.Dir(oldPath), err)
				}
				if err := os.Rename(newPath, oldPath); err != nil {
					return fmt.Errorf("restore appimage %q to %q: %w", newPath, oldPath, err)
				}
				return nil
			})
		}
		backup, err := updateDesktopEntry(plan.desktopEntryPath, plan.newAppImagePath, plan.id)
		if err != nil {
			return false, rollback.rollback(err)
		}
		if backup.changed {
			rollback.add(func() error {
				return restoreDesktopEntry(backup)
			})
		}
	}

	if err := writeDatabase(opts.DestPath, v2); err != nil {
		return false, rollback.rollback(err)
	}

	return true, nil
}

type legacyDatabase struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Apps          map[string]legacyAppRecord `json:"apps"`
}

type legacyAppRecord struct {
	Name             string             `json:"name"`
	ID               string             `json:"id"`
	Version          string             `json:"version"`
	ExecPath         string             `json:"exec_path"`
	IconPath         string             `json:"icon_path"`
	DesktopEntryPath string             `json:"desktop_entry_path"`
	DesktopEntryLink string             `json:"desktop_entry_link"`
	Source           legacySourceRecord `json:"source"`
	Update           legacyUpdateRecord `json:"update"`
}

type legacySourceRecord struct {
	Kind          string                    `json:"kind"`
	LocalFile     legacyLocalFileSource     `json:"local_file"`
	GitHubRelease legacyGitHubReleaseSource `json:"github_release"`
}

type legacyLocalFileSource struct {
	IntegratedAt string `json:"integrated_at"`
	OriginalPath string `json:"original_path"`
}

type legacyGitHubReleaseSource struct {
	Repo         string `json:"repo"`
	Asset        string `json:"asset"`
	Tag          string `json:"tag"`
	AssetName    string `json:"asset_name"`
	DownloadedAt string `json:"downloaded_at"`
}

type legacyUpdateRecord struct {
	Kind          string                    `json:"kind"`
	GitHubRelease legacyGitHubReleaseUpdate `json:"github_release"`
	Zsync         legacyZsyncUpdate         `json:"zsync"`
}

type legacyGitHubReleaseUpdate struct {
	Repo  string `json:"repo"`
	Asset string `json:"asset"`
}

type legacyZsyncUpdate struct {
	UpdateInfo string `json:"update_info"`
	Transport  string `json:"transport"`
}

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

type migrationPlan struct {
	id               string
	oldAppImagePath  string
	newAppImagePath  string
	desktopEntryPath string
	record           appRecord
}

func migrationPlans(legacy legacyDatabase, appImageDir string, desktopDir string) []migrationPlan {
	ids := make([]string, 0, len(legacy.Apps))
	for key := range legacy.Apps {
		ids = append(ids, key)
	}
	sort.Strings(ids)

	plans := make([]migrationPlan, 0, len(ids))
	for _, key := range ids {
		old := legacy.Apps[key]
		id := strings.TrimSpace(old.ID)
		if id == "" {
			id = key
		}
		newAppImagePath := filepath.Join(appImageDir, id+".AppImage")
		desktopEntryPath := strings.TrimSpace(old.DesktopEntryLink)
		if desktopEntryPath == "" {
			desktopEntryPath = filepath.Join(desktopDir, id+".desktop")
		}

		record := appRecord{
			ID:               id,
			Name:             old.Name,
			Version:          old.Version,
			AppImagePath:     newAppImagePath,
			DesktopEntryPath: desktopEntryPath,
			IconPath:         old.IconPath,
			Source:           migrateSource(old.Source),
			UpdateSource:     migrateUpdateSource(old.Update),
		}
		plans = append(plans, migrationPlan{
			id:               id,
			oldAppImagePath:  old.ExecPath,
			newAppImagePath:  newAppImagePath,
			desktopEntryPath: desktopEntryPath,
			record:           record,
		})
	}

	return plans
}

func migrateSource(old legacySourceRecord) *sourceRecord {
	switch strings.TrimSpace(old.Kind) {
	case "local_file":
		return &sourceRecord{
			Kind: "local",
			LocalFile: &localFileSourceRecord{
				Path:         old.LocalFile.OriginalPath,
				IntegratedAt: old.LocalFile.IntegratedAt,
			},
		}
	case "github_release":
		asset := strings.TrimSpace(old.GitHubRelease.AssetName)
		if asset == "" {
			asset = old.GitHubRelease.Asset
		}
		return &sourceRecord{
			Kind: "github",
			GitHubRelease: &githubReleaseSourceRecord{
				Repo:         old.GitHubRelease.Repo,
				Tag:          old.GitHubRelease.Tag,
				Asset:        asset,
				DownloadedAt: old.GitHubRelease.DownloadedAt,
			},
		}
	default:
		return nil
	}
}

func migrateUpdateSource(old legacyUpdateRecord) *updateSourceRecord {
	switch strings.TrimSpace(old.Kind) {
	case "github_release":
		return &updateSourceRecord{
			Kind:         "github",
			Repo:         old.GitHubRelease.Repo,
			AssetPattern: old.GitHubRelease.Asset,
		}
	case "zsync":
		return updateSourceFromZsync(old.Zsync.UpdateInfo, old.Zsync.Transport)
	default:
		return nil
	}
}

func updateSourceFromZsync(updateInfo string, fallbackTransport string) *updateSourceRecord {
	updateInfo = strings.TrimSpace(updateInfo)
	if updateInfo == "" {
		return nil
	}

	parts := strings.Split(updateInfo, "|")
	transport := strings.TrimSpace(parts[0])
	record := &updateSourceRecord{
		Embedded:  true,
		Kind:      "unsupported",
		Raw:       updateInfo,
		Transport: transport,
	}
	if record.Transport == "" {
		record.Transport = strings.TrimSpace(fallbackTransport)
	}

	switch transport {
	case "gh-releases-zsync":
		if len(parts) != 5 {
			return record
		}
		owner := strings.TrimSpace(parts[1])
		repo := strings.TrimSpace(parts[2])
		releaseTag := strings.TrimSpace(parts[3])
		zsyncPattern := strings.TrimSpace(parts[4])
		if owner == "" || repo == "" || releaseTag == "" || zsyncPattern == "" {
			return record
		}
		record.Kind = "github"
		record.Repo = owner + "/" + repo
		record.ReleaseTag = releaseTag
		record.ZsyncAssetPattern = zsyncPattern
		record.AssetPattern = strings.TrimSuffix(zsyncPattern, ".zsync")
		return record
	case "zsync":
		if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
			record.Kind = "zsync"
			record.URL = strings.TrimSpace(parts[1])
		}
		return record
	default:
		return record
	}
}

func readLegacyDatabase(path string) (legacyDatabase, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return legacyDatabase{}, fmt.Errorf("read legacy database %q: %w", path, err)
	}

	var db legacyDatabase
	if err := json.Unmarshal(bytes, &db); err != nil {
		return legacyDatabase{}, fmt.Errorf("parse legacy database %q: %w", path, err)
	}
	return db, nil
}

var writeDatabase = writeDatabaseFile

type migrationRollback struct {
	steps []func() error
}

func (r *migrationRollback) add(step func() error) {
	r.steps = append(r.steps, step)
}

func (r *migrationRollback) rollback(originalErr error) error {
	if len(r.steps) == 0 {
		return originalErr
	}

	var rollbackErrs []error
	for i := len(r.steps) - 1; i >= 0; i-- {
		if err := r.steps[i](); err != nil {
			rollbackErrs = append(rollbackErrs, err)
		}
	}
	if len(rollbackErrs) > 0 {
		return fmt.Errorf("%w; rollback failed: %w", originalErr, errors.Join(rollbackErrs...))
	}
	return originalErr
}

func preflightMigration(opts V1Options, plans []migrationPlan) error {
	if err := os.MkdirAll(filepath.Dir(opts.DestPath), 0o755); err != nil {
		return fmt.Errorf("create database directory %q: %w", filepath.Dir(opts.DestPath), err)
	}
	if info, err := os.Stat(opts.DestPath); err == nil {
		if info.IsDir() {
			return fmt.Errorf("database destination %q is a directory", opts.DestPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat database %q: %w", opts.DestPath, err)
	}

	for _, plan := range plans {
		src := strings.TrimSpace(plan.oldAppImagePath)
		dst := strings.TrimSpace(plan.newAppImagePath)
		if src == "" || dst == "" || src == dst {
			continue
		}
		if _, err := os.Stat(src); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("stat appimage %q: %w", src, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("create appimage directory %q: %w", filepath.Dir(dst), err)
		}
		if _, err := os.Stat(dst); err == nil {
			return fmt.Errorf("destination appimage %q already exists", dst)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat destination appimage %q: %w", dst, err)
		}
	}
	return nil
}

func writeDatabaseFile(path string, db databaseFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create database directory %q: %w", filepath.Dir(path), err)
	}

	bytes, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return fmt.Errorf("encode database: %w", err)
	}
	bytes = append(bytes, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0o644); err != nil {
		return fmt.Errorf("write temporary database %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace database %q: %w", path, err)
	}
	return nil
}

func moveIfNeeded(src string, dst string) (bool, error) {
	src = strings.TrimSpace(src)
	dst = strings.TrimSpace(dst)
	if src == "" || dst == "" || src == dst {
		return false, nil
	}

	if _, err := os.Stat(src); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat appimage %q: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, fmt.Errorf("create appimage directory %q: %w", filepath.Dir(dst), err)
	}
	if _, err := os.Stat(dst); err == nil {
		return false, fmt.Errorf("destination appimage %q already exists", dst)
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("stat destination appimage %q: %w", dst, err)
	}
	if err := os.Rename(src, dst); err != nil {
		return false, fmt.Errorf("move appimage %q to %q: %w", src, dst, err)
	}
	return true, nil
}

type desktopEntryBackup struct {
	path    string
	bytes   []byte
	mode    os.FileMode
	changed bool
}

func updateDesktopEntry(path string, appImagePath string, appID string) (desktopEntryBackup, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return desktopEntryBackup{}, nil
	}

	bytes, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return desktopEntryBackup{}, nil
		}
		return desktopEntryBackup{}, fmt.Errorf("read desktop entry %q: %w", path, err)
	}

	lines := strings.Split(string(bytes), "\n")
	changed := false
	for i, line := range lines {
		if strings.HasPrefix(line, "Exec=") {
			lines[i] = "Exec=" + appImagePath
			changed = true
			continue
		}
		if strings.HasPrefix(line, "Icon=") {
			lines[i] = "Icon=" + appID
			changed = true
		}
	}
	if !changed {
		return desktopEntryBackup{}, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return desktopEntryBackup{}, fmt.Errorf("stat desktop entry %q: %w", path, err)
	}
	backup := desktopEntryBackup{
		path:    path,
		bytes:   append([]byte(nil), bytes...),
		mode:    info.Mode().Perm(),
		changed: true,
	}

	content := strings.Join(lines, "\n")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), info.Mode().Perm()); err != nil {
		return desktopEntryBackup{}, fmt.Errorf("write temporary desktop entry %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return desktopEntryBackup{}, fmt.Errorf("replace desktop entry %q: %w", path, err)
	}
	return backup, nil
}

func restoreDesktopEntry(backup desktopEntryBackup) error {
	if !backup.changed {
		return nil
	}
	tmp := backup.path + ".rollback.tmp"
	if err := os.WriteFile(tmp, backup.bytes, backup.mode.Perm()); err != nil {
		return fmt.Errorf("write rollback desktop entry %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, backup.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("restore desktop entry %q: %w", backup.path, err)
	}
	return nil
}
