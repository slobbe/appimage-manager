package repo

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/slobbe/appimage-manager/internal/config"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	models "github.com/slobbe/appimage-manager/internal/types"
)

type migrationSource struct {
	Name               string
	AppDir             string
	ConfigDir          string
	DBPath             string
	PathRoots          []string
	DBTieBreakPriority int
}

type migrationSourceState struct {
	Source       migrationSource
	DBExists     bool
	DBModTime    time.Time
	ConfigExists bool
}

type treeEntry struct {
	sourcePath string
	info       os.FileInfo
}

func MigrateToCurrentPaths() error {
	_, err := MigrateToCurrentPathsChanged()
	return err
}

func MigrateToCurrentPathsChanged() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}

	sourceStates, err := discoverMigrationSources(home)
	if err != nil {
		return false, err
	}

	destExists, err := fileExists(config.DbSrc)
	if err != nil {
		return false, err
	}

	orderedStates := orderedLegacyStates(sourceStates)
	if err := migrateConfigTree(orderedStates); err != nil {
		return false, fmt.Errorf("failed to migrate config directory: %w", err)
	}

	orderedSources := extractMigrationSources(orderedStates)

	if destExists {
		destDB, err := LoadDB(config.DbSrc)
		if err != nil {
			return false, fmt.Errorf("failed to load current database: %w", err)
		}

		changed, err := repairCurrentDB(destDB, orderedSources)
		if err != nil {
			return false, err
		}
		if !changed {
			return false, nil
		}
		if err := SaveDB(config.DbSrc, destDB); err != nil {
			return false, fmt.Errorf("failed to write current database: %w", err)
		}
		refreshMigratedDesktopIntegrationCaches()
		return true, nil
	}

	canonicalState := chooseCanonicalLegacyDB(orderedStates)
	if canonicalState == nil {
		return false, nil
	}

	destDB, err := LoadDB(canonicalState.Source.DBPath)
	if err != nil {
		return false, fmt.Errorf("failed to load %s database: %w", canonicalState.Source.Name, err)
	}

	changed, err := repairCurrentDB(destDB, orderedSources)
	if err != nil {
		return false, err
	}

	if err := SaveDB(config.DbSrc, destDB); err != nil {
		return false, fmt.Errorf("failed to write current database: %w", err)
	}
	refreshMigratedDesktopIntegrationCaches()

	return changed, nil
}

func MigrateAppToCurrentPaths(id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, fmt.Errorf("id cannot be empty")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}

	sourceStates, err := discoverMigrationSources(home)
	if err != nil {
		return false, err
	}

	orderedStates := orderedLegacyStates(sourceStates)
	if err := migrateConfigTree(orderedStates); err != nil {
		return false, fmt.Errorf("failed to migrate config directory: %w", err)
	}

	destExists, err := fileExists(config.DbSrc)
	if err != nil {
		return false, err
	}
	if !destExists {
		return false, fmt.Errorf("%s does not exists in database", id)
	}

	db, err := LoadDB(config.DbSrc)
	if err != nil {
		return false, fmt.Errorf("failed to load current database: %w", err)
	}

	changed, err := repairCurrentApp(db, extractMigrationSources(orderedStates), id)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}

	if err := SaveDB(config.DbSrc, db); err != nil {
		return false, fmt.Errorf("failed to write current database: %w", err)
	}
	refreshMigratedDesktopIntegrationCaches()
	return true, nil
}

func migrationSources(home string) []migrationSource {
	currentDataHome := filepath.Dir(config.AimDir)
	currentConfigHome := filepath.Dir(config.ConfigDir)
	currentStateHome := filepath.Dir(filepath.Dir(config.DbSrc))

	legacyAimDir := filepath.Join(home, ".appimage-manager")
	oldXDGAimDir := filepath.Join(currentDataHome, "appimage-manager")

	return []migrationSource{
		{
			Name:               "legacy home",
			AppDir:             legacyAimDir,
			DBPath:             filepath.Join(legacyAimDir, "apps.json"),
			PathRoots:          []string{legacyAimDir},
			DBTieBreakPriority: 0,
		},
		{
			Name:               "legacy xdg",
			AppDir:             oldXDGAimDir,
			ConfigDir:          filepath.Join(currentConfigHome, "appimage-manager"),
			DBPath:             filepath.Join(currentStateHome, "appimage-manager", "apps.json"),
			PathRoots:          []string{oldXDGAimDir},
			DBTieBreakPriority: 1,
		},
	}
}

func discoverMigrationSources(home string) ([]migrationSourceState, error) {
	sources := migrationSources(home)
	states := make([]migrationSourceState, 0, len(sources))

	for _, source := range sources {
		dbExists, dbModTime, err := migrationDBInfo(source.DBPath)
		if err != nil {
			return nil, fmt.Errorf("failed to inspect %s database: %w", source.Name, err)
		}

		configExists, err := dirExists(source.ConfigDir)
		if err != nil {
			return nil, fmt.Errorf("failed to inspect %s config directory: %w", source.Name, err)
		}

		if !dbExists && !configExists {
			continue
		}

		states = append(states, migrationSourceState{
			Source:       source,
			DBExists:     dbExists,
			DBModTime:    dbModTime,
			ConfigExists: configExists,
		})
	}

	return states, nil
}

func chooseCanonicalLegacyDB(states []migrationSourceState) *migrationSourceState {
	for i := range states {
		if states[i].DBExists {
			return &states[i]
		}
	}
	return nil
}

func orderedLegacyStates(states []migrationSourceState) []migrationSourceState {
	ordered := append([]migrationSourceState(nil), states...)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := ordered[i]
		right := ordered[j]

		if left.DBExists != right.DBExists {
			return left.DBExists
		}
		if left.DBExists && right.DBExists {
			if !left.DBModTime.Equal(right.DBModTime) {
				return left.DBModTime.After(right.DBModTime)
			}
		}
		if left.Source.DBTieBreakPriority != right.Source.DBTieBreakPriority {
			return left.Source.DBTieBreakPriority > right.Source.DBTieBreakPriority
		}

		return left.Source.Name < right.Source.Name
	})
	return ordered
}

func extractMigrationSources(states []migrationSourceState) []migrationSource {
	sources := make([]migrationSource, 0, len(states))
	for _, state := range states {
		sources = append(sources, state.Source)
	}
	return sources
}

func migrateConfigTree(states []migrationSourceState) error {
	var roots []string
	for _, state := range states {
		if !state.ConfigExists || strings.TrimSpace(state.Source.ConfigDir) == "" {
			continue
		}
		roots = append(roots, state.Source.ConfigDir)
	}

	return mergeTreePreferFirst(config.ConfigDir, roots)
}

func migrationDBInfo(path string) (bool, time.Time, error) {
	if strings.TrimSpace(path) == "" {
		return false, time.Time{}, nil
	}

	info, err := os.Stat(path)
	if err == nil {
		if info.IsDir() {
			return false, time.Time{}, nil
		}
		return true, info.ModTime(), nil
	}
	if os.IsNotExist(err) {
		return false, time.Time{}, nil
	}
	return false, time.Time{}, err
}

func repairCurrentDB(db *DB, sources []migrationSource) (bool, error) {
	if db == nil {
		return false, nil
	}

	changed := false
	if db.Apps == nil {
		db.Apps = map[string]*models.App{}
		changed = true
	}
	if db.SchemaVersion == 0 {
		db.SchemaVersion = 1
		changed = true
	}

	keys := make([]string, 0, len(db.Apps))
	for key := range db.Apps {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	updatedApps := make(map[string]*models.App, len(db.Apps))

	for _, key := range keys {
		app := db.Apps[key]
		if app == nil {
			continue
		}
		if strings.TrimSpace(app.ID) == "" {
			app.ID = strings.TrimSpace(key)
			if app.ID == "" {
				continue
			}
			changed = true
		}

		updated, err := reconcileAppToCurrentPaths(app, nil, sources, db.Apps, updatedApps)
		if err != nil {
			return false, err
		}
		changed = changed || updated
		if key != app.ID {
			changed = true
		}
		updatedApps[app.ID] = app
	}

	db.Apps = updatedApps

	return changed, nil
}

func repairCurrentApp(db *DB, sources []migrationSource, id string) (bool, error) {
	if db == nil {
		return false, nil
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return false, fmt.Errorf("id cannot be empty")
	}

	changed := false
	if db.Apps == nil {
		db.Apps = map[string]*models.App{}
		changed = true
	}
	if db.SchemaVersion == 0 {
		db.SchemaVersion = 1
		changed = true
	}

	app, ok := db.Apps[id]
	if !ok || app == nil {
		return false, fmt.Errorf("%s does not exists in database", id)
	}
	if strings.TrimSpace(app.ID) == "" {
		app.ID = id
		changed = true
	}

	updated, err := reconcileAppToCurrentPaths(app, nil, sources, db.Apps, map[string]*models.App{})
	if err != nil {
		return false, err
	}
	changed = changed || updated

	if app.ID != id {
		delete(db.Apps, id)
		db.Apps[app.ID] = app
		changed = true
	}

	return changed, nil
}

func reconcileAppToCurrentPaths(app, fallback *models.App, sources []migrationSource, existingApps, updatedApps map[string]*models.App) (bool, error) {
	if app == nil || strings.TrimSpace(app.ID) == "" {
		return false, nil
	}

	changed := false
	preRenameIconPath := strings.TrimSpace(app.IconPath)
	preRenameID := strings.TrimSpace(app.ID)

	renamed, err := migrateAppIdentityToUpstreamDesktopStem(app, existingApps, updatedApps)
	if err != nil {
		return false, err
	}
	changed = changed || renamed

	if err := mergeAppDirFromSources(app.ID, sources); err != nil {
		return false, err
	}

	if fillMissingAppPathsFromFallback(app, fallback, sources) {
		changed = true
	}

	if rewriteAppPathsToCurrent(app, allPathRoots(sources)) {
		changed = true
	}

	iconCandidates := iconCandidatePaths(app.ID, app, fallback, sources)
	if preRenameIconPath != "" {
		iconCandidates = append(iconCandidates, preRenameIconPath)
	}
	if preRenameID != "" && preRenameID != app.ID {
		iconCandidates = append(iconCandidates, managedIconRepairCandidates(preRenameID, app, fallback)...)
	}
	installedIconPath, desktopIconValue, iconChanged, err := installMigratedIcon(app.ID, app.IconPath, iconCandidates...)
	if err != nil {
		return false, err
	}
	if iconChanged {
		if app.IconPath != installedIconPath {
			app.IconPath = installedIconPath
		}
		changed = true
	}

	if err := rewriteCurrentDesktopEntry(app.DesktopEntryPath, app.ExecPath, desktopIconValue); err != nil {
		return false, err
	}

	linkChanged, err := reconcileDesktopLink(app, fallback)
	if err != nil {
		return false, err
	}
	changed = changed || linkChanged

	return changed, nil
}

func mergeAppDirFromSources(appID string, sources []migrationSource) error {
	var roots []string
	for _, source := range sources {
		if strings.TrimSpace(source.AppDir) == "" {
			continue
		}
		roots = append(roots, filepath.Join(source.AppDir, appID))
	}

	return mergeTreePreferFirst(filepath.Join(config.AimDir, appID), roots)
}

func fillMissingAppPathsFromFallback(app, fallback *models.App, sources []migrationSource) bool {
	if app == nil || fallback == nil {
		return false
	}

	changed := false
	roots := allPathRoots(sources)

	if strings.TrimSpace(app.ExecPath) == "" {
		if value := rewriteValueToCurrent(fallback.ExecPath, roots); value != "" {
			app.ExecPath = value
			changed = true
		}
	}
	if strings.TrimSpace(app.DesktopEntryPath) == "" {
		if value := rewriteValueToCurrent(fallback.DesktopEntryPath, roots); value != "" {
			app.DesktopEntryPath = value
			changed = true
		}
	}
	if strings.TrimSpace(app.IconPath) == "" {
		if value := rewriteValueToCurrent(fallback.IconPath, roots); value != "" {
			app.IconPath = value
			changed = true
		}
	}
	if strings.TrimSpace(app.DesktopEntryLink) == "" && strings.TrimSpace(fallback.DesktopEntryLink) != "" {
		app.DesktopEntryLink = fallback.DesktopEntryLink
		changed = true
	}

	if app.Source.Kind == models.SourceLocalFile {
		switch {
		case app.Source.LocalFile == nil && fallback.Source.LocalFile != nil:
			copyValue := *fallback.Source.LocalFile
			copyValue.OriginalPath = rewriteValueToCurrent(copyValue.OriginalPath, roots)
			app.Source.LocalFile = &copyValue
			changed = true
		case app.Source.LocalFile != nil && strings.TrimSpace(app.Source.LocalFile.OriginalPath) == "" && fallback.Source.LocalFile != nil:
			if value := rewriteValueToCurrent(fallback.Source.LocalFile.OriginalPath, roots); value != "" {
				app.Source.LocalFile.OriginalPath = value
				changed = true
			}
		}
	}

	return changed
}

func reconcileDesktopLink(app, fallback *models.App) (bool, error) {
	if app == nil {
		return false, nil
	}

	if strings.TrimSpace(app.DesktopEntryPath) == "" {
		return false, nil
	}

	changed := false
	expectedLink, err := util.ResolveDesktopLinkPath(config.DesktopDir, app.DesktopEntryPath, app.ID+".desktop", "aim-"+app.ID+".desktop")
	if err != nil {
		return false, err
	}
	if app.DesktopEntryLink != expectedLink {
		if owned, exists, err := util.DesktopLinkOwnedBy(app.DesktopEntryLink, app.DesktopEntryPath); err == nil && owned && exists {
			_ = os.Remove(app.DesktopEntryLink)
		}
		app.DesktopEntryLink = expectedLink
		changed = true
	}

	if _, err := os.Stat(app.DesktopEntryPath); err == nil {
		if err := ensureDesktopLink(app.DesktopEntryPath, app.DesktopEntryLink); err != nil {
			return false, err
		}
		return changed, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}

	if app.DesktopEntryLink != "" {
		app.DesktopEntryLink = ""
		return true, nil
	}

	return changed, nil
}

func rewriteCurrentDesktopEntry(path, execPath, iconValue string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}

	if _, err := os.Stat(path); err == nil {
		return util.RewriteDesktopEntryFile(path, execPath, iconValue)
	} else if !os.IsNotExist(err) {
		return err
	}

	return nil
}

func rewriteAppPathsToCurrent(app *models.App, roots []string) bool {
	if app == nil {
		return false
	}

	changed := false

	newExecPath := rewriteValueToCurrent(app.ExecPath, roots)
	if newExecPath != app.ExecPath {
		app.ExecPath = newExecPath
		changed = true
	}

	newDesktopPath := rewriteValueToCurrent(app.DesktopEntryPath, roots)
	if newDesktopPath != app.DesktopEntryPath {
		app.DesktopEntryPath = newDesktopPath
		changed = true
	}

	newIconPath := rewriteValueToCurrent(app.IconPath, roots)
	if newIconPath != app.IconPath {
		app.IconPath = newIconPath
		changed = true
	}

	if app.Source.LocalFile != nil {
		newOriginalPath := rewriteValueToCurrent(app.Source.LocalFile.OriginalPath, roots)
		if newOriginalPath != app.Source.LocalFile.OriginalPath {
			app.Source.LocalFile.OriginalPath = newOriginalPath
			changed = true
		}
	}

	return changed
}

func rewriteValueToCurrent(value string, roots []string) string {
	rewritten := value
	for _, root := range roots {
		rewritten = rewritePrefix(rewritten, root, config.AimDir)
	}
	return rewritten
}

func allPathRoots(sources []migrationSource) []string {
	seen := map[string]struct{}{}
	var roots []string

	for _, source := range sources {
		for _, root := range source.PathRoots {
			trimmed := strings.TrimSpace(root)
			if trimmed == "" {
				continue
			}
			clean := filepath.Clean(trimmed)
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			roots = append(roots, clean)
		}
	}

	return roots
}

func iconCandidatePaths(appID string, app, fallback *models.App, sources []migrationSource) []string {
	paths := []string{app.IconPath}
	if fallback != nil {
		paths = append(paths, fallback.IconPath)
	}

	paths = append(paths, managedIconRepairCandidates(util.Slugify(app.Name), app, fallback)...)

	for _, root := range allPathRoots(sources) {
		paths = append(paths, deriveSourcePath(app.IconPath, root))
		paths = append(paths, deriveAppDirIconPath(appID, root, app.IconPath))
		if fallback != nil {
			paths = append(paths, deriveSourcePath(fallback.IconPath, root))
			paths = append(paths, deriveAppDirIconPath(appID, root, fallback.IconPath))
		}
	}

	return paths
}

func managedIconRepairCandidates(candidateID string, app, fallback *models.App) []string {
	candidateID = strings.TrimSpace(candidateID)
	if candidateID == "" {
		return nil
	}

	var paths []string
	for _, iconPath := range []string{strings.TrimSpace(app.IconPath), strings.TrimSpace(pathOrEmpty(fallback))} {
		ext := strings.ToLower(filepath.Ext(iconPath))
		if ext == "" {
			continue
		}
		paths = append(paths, filepath.Join(iconInstallDir(ext), candidateID+ext))
	}

	return paths
}

func pathOrEmpty(app *models.App) string {
	if app == nil {
		return ""
	}
	return app.IconPath
}

func deriveAppDirIconPath(appID, sourceRoot, iconPath string) string {
	if strings.TrimSpace(appID) == "" || strings.TrimSpace(sourceRoot) == "" {
		return ""
	}

	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(iconPath)))
	if ext == "" {
		return ""
	}

	return filepath.Join(filepath.Clean(sourceRoot), appID, appID+ext)
}

func migrateAppIdentityToUpstreamDesktopStem(app *models.App, existingApps, updatedApps map[string]*models.App) (bool, error) {
	if app == nil {
		return false, nil
	}

	oldID := strings.TrimSpace(app.ID)
	if oldID == "" {
		return false, nil
	}

	newID := desiredManagedAppID(app)
	if newID == "" || newID == oldID {
		return false, nil
	}

	if existing, ok := existingApps[newID]; ok && existing != nil && existing != app {
		return false, nil
	}
	if existing, ok := updatedApps[newID]; ok && existing != nil && existing != app {
		return false, nil
	}

	oldAppDir := filepath.Join(config.AimDir, oldID)
	newAppDir := filepath.Join(config.AimDir, newID)

	if _, err := os.Stat(newAppDir); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}

	if _, err := os.Stat(oldAppDir); err == nil {
		if err := os.Rename(oldAppDir, newAppDir); err != nil {
			return false, err
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}

	app.ID = newID
	rewriteOwnedAppPathsForIDChange(app, oldID, newID)
	if err := renameManagedAppFilesForIDChange(app, oldID, newID); err != nil {
		return false, err
	}

	if owned, exists, err := util.DesktopLinkOwnedBy(app.DesktopEntryLink, app.DesktopEntryPath); err == nil && owned && exists {
		_ = os.Remove(app.DesktopEntryLink)
	}
	app.DesktopEntryLink = ""

	return true, nil
}

func desiredManagedAppID(app *models.App) string {
	if app == nil {
		return ""
	}

	if stem := upstreamDesktopStemFromAppImage(app.ExecPath); stem != "" {
		return stem
	}

	if stem := util.SanitizeDesktopStem(util.DesktopStemFromPath(app.DesktopEntryPath)); stem != "" {
		return stem
	}

	return strings.TrimSpace(app.ID)
}

func rewriteOwnedAppPathsForIDChange(app *models.App, oldID, newID string) {
	if app == nil {
		return
	}

	oldDir := filepath.Join(config.AimDir, oldID)
	newDir := filepath.Join(config.AimDir, newID)

	app.ExecPath = rewritePrefix(app.ExecPath, oldDir, newDir)
	app.DesktopEntryPath = rewritePrefix(app.DesktopEntryPath, oldDir, newDir)
	app.IconPath = rewriteOwnedIconPathForIDChange(app.IconPath, oldID, newID)

	if app.Source.LocalFile != nil {
		app.Source.LocalFile.OriginalPath = rewritePrefix(app.Source.LocalFile.OriginalPath, oldDir, newDir)
	}
}

func rewriteOwnedIconPathForIDChange(path, oldID, newID string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}

	ext := filepath.Ext(trimmed)
	if ext == "" {
		return trimmed
	}

	if filepath.Base(trimmed) != oldID+ext {
		return trimmed
	}

	dir := filepath.Dir(trimmed)
	if !isManagedIconDir(dir) {
		return trimmed
	}

	return filepath.Join(dir, newID+ext)
}

func renameManagedAppFilesForIDChange(app *models.App, oldID, newID string) error {
	if app == nil {
		return nil
	}

	var err error
	app.ExecPath, err = renameManagedFileWithIDChange(app.ExecPath, oldID, newID)
	if err != nil {
		return err
	}
	app.DesktopEntryPath, err = renameManagedFileWithIDChange(app.DesktopEntryPath, oldID, newID)
	if err != nil {
		return err
	}
	app.IconPath, err = renameManagedFileWithIDChange(app.IconPath, oldID, newID)
	if err != nil {
		return err
	}

	if app.Source.LocalFile != nil {
		app.Source.LocalFile.OriginalPath, err = renameManagedFileWithIDChange(app.Source.LocalFile.OriginalPath, oldID, newID)
		if err != nil {
			return err
		}
	}

	return nil
}

func renameManagedFileWithIDChange(path, oldID, newID string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return path, nil
	}

	ext := filepath.Ext(trimmed)
	if ext == "" {
		return path, nil
	}

	if filepath.Base(trimmed) != oldID+ext {
		return path, nil
	}

	dir := filepath.Dir(trimmed)
	cleanDir := filepath.Clean(dir)
	if !strings.HasPrefix(cleanDir, filepath.Clean(config.AimDir)+string(filepath.Separator)) && !isManagedIconDir(cleanDir) {
		return path, nil
	}

	target := filepath.Join(dir, newID+ext)
	if filepath.Clean(trimmed) == filepath.Clean(target) {
		return target, nil
	}

	if _, err := os.Stat(trimmed); err != nil {
		if os.IsNotExist(err) {
			return target, nil
		}
		return path, err
	}
	if _, err := os.Stat(target); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return path, err
	}

	if err := os.Rename(trimmed, target); err != nil {
		return path, err
	}

	return target, nil
}

func isManagedIconDir(dir string) bool {
	cleanDir := filepath.Clean(dir)
	root := filepath.Clean(config.IconThemeDir)
	if cleanDir == root || strings.HasPrefix(cleanDir, root+string(filepath.Separator)) {
		return true
	}

	return false
}

func upstreamDesktopStemFromAppImage(appImagePath string) string {
	appImagePath = strings.TrimSpace(appImagePath)
	if appImagePath == "" || !strings.EqualFold(filepath.Ext(appImagePath), ".AppImage") {
		return ""
	}

	info, err := os.Stat(appImagePath)
	if err != nil || info.IsDir() {
		return ""
	}

	tmpDir, err := os.MkdirTemp("", "aim-migrate-*")
	if err != nil {
		return ""
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	if err := os.Chmod(appImagePath, 0o755); err != nil && !os.IsPermission(err) {
		return ""
	}

	cmd := exec.Command(appImagePath, "--appimage-extract")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		return ""
	}

	root := filepath.Join(tmpDir, "squashfs-root")
	desktopPath, err := locateDesktopFileForMigration(root)
	if err != nil {
		return ""
	}

	if resolved, err := filepath.EvalSymlinks(desktopPath); err == nil {
		desktopPath = resolved
	}

	return util.SanitizeDesktopStem(util.DesktopStemFromPath(desktopPath))
}

func locateDesktopFileForMigration(root string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(root, "*.desktop"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(d.Name()), ".desktop") {
				matches = append(matches, path)
			}
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	if len(matches) == 0 {
		return "", os.ErrNotExist
	}

	sort.Strings(matches)
	dirName := filepath.Base(root)
	for _, candidate := range matches {
		if util.DesktopStemFromPath(candidate) == dirName {
			return candidate, nil
		}
	}
	for _, candidate := range matches {
		rel, err := filepath.Rel(root, candidate)
		if err == nil && strings.HasPrefix(filepath.ToSlash(rel), "usr/share/applications/") {
			return candidate, nil
		}
	}

	return matches[0], nil
}

func deriveSourcePath(path, sourceRoot string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || strings.TrimSpace(sourceRoot) == "" {
		return ""
	}

	cleanValue := filepath.Clean(trimmed)
	cleanSourceRoot := filepath.Clean(sourceRoot)
	if strings.HasPrefix(cleanValue, cleanSourceRoot+string(filepath.Separator)) {
		return trimmed
	}

	rel, err := filepath.Rel(filepath.Clean(config.AimDir), cleanValue)
	if err != nil {
		return ""
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}

	return filepath.Join(cleanSourceRoot, rel)
}

func installMigratedIcon(appID, currentIconPath string, candidates ...string) (string, string, bool, error) {
	sourceIconPath := selectExistingPath(candidates...)
	if sourceIconPath == "" {
		if existing := selectExistingPath(currentIconPath); existing != "" {
			return existing, existing, filepath.Clean(currentIconPath) != filepath.Clean(existing), nil
		}
		return currentIconPath, "", false, nil
	}

	ext := strings.ToLower(filepath.Ext(sourceIconPath))
	if ext == "" {
		return currentIconPath, currentIconPath, false, nil
	}

	destinationDir := iconInstallDir(ext)
	destinationPath := filepath.Join(destinationDir, appID+ext)
	if err := os.MkdirAll(destinationDir, 0o755); err != nil {
		return "", "", false, err
	}

	if filepath.Clean(sourceIconPath) != filepath.Clean(destinationPath) {
		info, err := os.Stat(sourceIconPath)
		if err != nil {
			return "", "", false, err
		}
		if err := copyFile(sourceIconPath, destinationPath, info.Mode().Perm()); err != nil {
			return "", "", false, err
		}
	}

	return destinationPath, destinationPath, filepath.Clean(currentIconPath) != filepath.Clean(destinationPath), nil
}

func mergeTreePreferFirst(dst string, srcRoots []string) error {
	entries, err := collectRelativeEntries(srcRoots)
	if err != nil {
		return err
	}

	relPaths := make([]string, 0, len(entries))
	for relPath := range entries {
		relPaths = append(relPaths, relPath)
	}
	sort.Strings(relPaths)

	for _, relPath := range relPaths {
		dstPath := filepath.Join(dst, relPath)
		if _, err := os.Lstat(dstPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}

		if err := copyPreferredEntry(dstPath, entries[relPath]); err != nil {
			return err
		}
	}

	return nil
}

func collectRelativeEntries(srcRoots []string) (map[string]treeEntry, error) {
	entries := make(map[string]treeEntry)

	for _, root := range srcRoots {
		trimmed := strings.TrimSpace(root)
		if trimmed == "" {
			continue
		}
		if err := collectTreeEntries(trimmed, ".", entries); err != nil {
			return nil, err
		}
	}

	return entries, nil
}

func collectTreeEntries(root, relPath string, entries map[string]treeEntry) error {
	currentPath := root
	if relPath != "." {
		currentPath = filepath.Join(root, relPath)
	}

	info, err := os.Lstat(currentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if relPath != "." {
		if _, exists := entries[relPath]; !exists {
			entries[relPath] = treeEntry{
				sourcePath: currentPath,
				info:       info,
			}
		}
	}

	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil
	}

	dirEntries, err := os.ReadDir(currentPath)
	if err != nil {
		return err
	}
	for _, entry := range dirEntries {
		childRel := entry.Name()
		if relPath != "." {
			childRel = filepath.Join(relPath, entry.Name())
		}
		if err := collectTreeEntries(root, childRel, entries); err != nil {
			return err
		}
	}

	return nil
}

func copyPreferredEntry(dstPath string, entry treeEntry) error {
	if entry.info == nil {
		return nil
	}

	if entry.info.IsDir() {
		return os.MkdirAll(dstPath, entry.info.Mode().Perm())
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}

	if entry.info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(entry.sourcePath)
		if err != nil {
			return err
		}
		return os.Symlink(target, dstPath)
	}

	return copyFile(entry.sourcePath, dstPath, entry.info.Mode().Perm())
}

func selectExistingPath(paths ...string) string {
	seen := map[string]struct{}{}

	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			continue
		}
		clean := filepath.Clean(trimmed)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}

		if _, err := os.Stat(clean); err == nil {
			return clean
		}
	}

	return ""
}

func fileExists(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, nil
	}

	info, err := os.Stat(path)
	if err == nil {
		return !info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func dirExists(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, nil
	}

	info, err := os.Stat(path)
	if err == nil {
		return info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func iconInstallDir(ext string) string {
	if ext == ".svg" {
		return filepath.Join(config.IconThemeDir, "scalable", "apps")
	}
	return filepath.Join(config.IconThemeDir, "256x256", "apps")
}

func rewritePrefix(value, oldBase, newBase string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value
	}

	cleanValue := filepath.Clean(trimmed)
	cleanOld := filepath.Clean(oldBase)

	rel, err := filepath.Rel(cleanOld, cleanValue)
	if err != nil {
		return value
	}

	if rel == "." {
		return filepath.Clean(newBase)
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return value
	}

	return filepath.Join(newBase, rel)
}

func ensureDesktopLink(target, linkPath string) error {
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return err
	}

	_ = os.Remove(linkPath)
	if err := os.Symlink(target, linkPath); err != nil {
		return err
	}

	return nil
}

func copyDirMissing(srcDir, dstDir string) error {
	if filepath.Clean(srcDir) == filepath.Clean(dstDir) {
		return nil
	}

	info, err := os.Stat(srcDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}

	if err := os.MkdirAll(dstDir, info.Mode().Perm()); err != nil {
		return err
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())

		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if err := copyDirMissing(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}

		if entryInfo.Mode()&os.ModeSymlink != 0 {
			if _, err := os.Lstat(dstPath); err == nil {
				continue
			} else if !os.IsNotExist(err) {
				return err
			}

			target, err := os.Readlink(srcPath)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, dstPath); err != nil {
				return err
			}
			continue
		}

		if _, err := os.Stat(dstPath); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return err
		}

		if err := copyFile(srcPath, dstPath, entryInfo.Mode().Perm()); err != nil {
			return err
		}
	}

	return nil
}

func refreshMigratedDesktopIntegrationCaches() {
	ctx := context.Background()

	if _, err := runMigrationCommandIfAvailable(ctx, "update-desktop-database", config.DesktopDir); err != nil {
		return
	}

	ranXDG, err := runMigrationCommandIfAvailable(ctx, "xdg-icon-resource", "forceupdate")
	if err != nil {
		return
	}
	if ranXDG {
		return
	}

	_, _ = runMigrationCommandIfAvailable(ctx, "gtk-update-icon-cache", "-f", config.IconThemeDir)
}

func runMigrationCommandIfAvailable(ctx context.Context, name string, args ...string) (bool, error) {
	binary, err := exec.LookPath(name)
	if err != nil {
		if err == exec.ErrNotFound {
			return false, nil
		}
		return false, err
	}

	if err := exec.CommandContext(ctx, binary, args...).Run(); err != nil {
		return true, err
	}

	return true, nil
}

func copyFile(srcPath, dstPath string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	return nil
}
