package repo

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/slobbe/appimage-manager/internal/config"
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
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	sourceStates, err := discoverMigrationSources(home)
	if err != nil {
		return err
	}

	destExists, err := fileExists(config.DbSrc)
	if err != nil {
		return err
	}

	orderedStates := orderedLegacyStates(sourceStates)
	if err := migrateConfigTree(orderedStates); err != nil {
		return fmt.Errorf("failed to migrate config directory: %w", err)
	}

	orderedSources := extractMigrationSources(orderedStates)

	if destExists {
		destDB, err := LoadDB(config.DbSrc)
		if err != nil {
			return fmt.Errorf("failed to load current database: %w", err)
		}

		changed, err := repairCurrentDB(destDB, orderedSources)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		if err := SaveDB(config.DbSrc, destDB); err != nil {
			return fmt.Errorf("failed to write current database: %w", err)
		}
		return nil
	}

	canonicalState := chooseCanonicalLegacyDB(orderedStates)
	if canonicalState == nil {
		return nil
	}

	destDB, err := LoadDB(canonicalState.Source.DBPath)
	if err != nil {
		return fmt.Errorf("failed to load %s database: %w", canonicalState.Source.Name, err)
	}

	if _, err := repairCurrentDB(destDB, orderedSources); err != nil {
		return err
	}

	if err := SaveDB(config.DbSrc, destDB); err != nil {
		return fmt.Errorf("failed to write current database: %w", err)
	}

	return nil
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

	for key, app := range db.Apps {
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

		updated, err := reconcileAppToCurrentPaths(app, nil, sources)
		if err != nil {
			return false, err
		}
		changed = changed || updated
	}

	return changed, nil
}

func reconcileAppToCurrentPaths(app, fallback *models.App, sources []migrationSource) (bool, error) {
	if app == nil || strings.TrimSpace(app.ID) == "" {
		return false, nil
	}

	changed := false

	if err := mergeAppDirFromSources(app.ID, sources); err != nil {
		return false, err
	}

	if fillMissingAppPathsFromFallback(app, fallback, sources) {
		changed = true
	}

	if rewriteAppPathsToCurrent(app, allPathRoots(sources)) {
		changed = true
	}

	installedIconPath, desktopIconValue, iconChanged, err := installMigratedIcon(app.ID, app.IconPath, iconCandidatePaths(app.ID, app, fallback, sources)...)
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

	linkCandidate := strings.TrimSpace(app.DesktopEntryLink)
	if linkCandidate == "" && fallback != nil {
		linkCandidate = strings.TrimSpace(fallback.DesktopEntryLink)
	}
	if linkCandidate == "" {
		return false, nil
	}

	linkName := filepath.Base(linkCandidate)
	if linkName == "" || linkName == "." || linkName == string(filepath.Separator) {
		if app.DesktopEntryLink != "" {
			app.DesktopEntryLink = ""
			return true, nil
		}
		return false, nil
	}

	changed := false
	expectedLink := filepath.Join(config.DesktopDir, linkName)
	if app.DesktopEntryLink != expectedLink {
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
		return rewriteDesktopEntryFile(path, execPath, iconValue)
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
		return currentIconPath, currentIconPath, false, nil
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
	if isThemeLookupExtension(ext) {
		return filepath.Join(config.IconThemeDir, "256x256", "apps")
	}
	return config.PixmapsDir
}

func isThemeLookupExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".svg", ".xpm":
		return true
	default:
		return false
	}
}

func rewriteDesktopEntryFile(path, execPath, iconValue string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	inDesktopEntryGroup := false
	inDesktopActionGroup := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inDesktopEntryGroup = trimmed == "[Desktop Entry]"
			inDesktopActionGroup = strings.HasPrefix(trimmed, "[Desktop Action ") && strings.HasSuffix(trimmed, "]")
			continue
		}

		if !inDesktopEntryGroup && !inDesktopActionGroup {
			continue
		}

		if strings.HasPrefix(trimmed, "Exec=") {
			lines[i] = rewriteExecLine(trimmed, execPath)
		}

		if inDesktopEntryGroup && iconValue != "" && strings.HasPrefix(trimmed, "Icon=") {
			lines[i] = "Icon=" + iconValue
		}
	}

	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}

	fileInfo, statErr := os.Stat(path)
	perm := os.FileMode(0o644)
	if statErr == nil {
		perm = fileInfo.Mode().Perm() & 0o666
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), perm)
}

func rewriteExecLine(execLine, execPath string) string {
	value := strings.TrimPrefix(execLine, "Exec=")
	value = strings.TrimSpace(value)
	_, args := splitDesktopExec(value)
	return "Exec=" + quoteDesktopExecArg(execPath) + args
}

func splitDesktopExec(value string) (string, string) {
	if value == "" {
		return "", ""
	}

	if value[0] == '"' {
		escaped := false
		for i := 1; i < len(value); i++ {
			if escaped {
				escaped = false
				continue
			}
			if value[i] == '\\' {
				escaped = true
				continue
			}
			if value[i] == '"' {
				return value[:i+1], value[i+1:]
			}
		}
		return value, ""
	}

	if idx := strings.IndexAny(value, " \t"); idx >= 0 {
		return value[:idx], value[idx:]
	}

	return value, ""
}

func quoteDesktopExecArg(value string) string {
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, " \t\n\r\"") {
		return strconv.Quote(value)
	}
	return value
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
