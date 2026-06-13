package icon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/slobbe/appimage-manager/internal/app"
)

var supportedIconExtensions = map[string]struct{}{
	".png":  {},
	".svg":  {},
	".svgz": {},
	".xpm":  {},
	".ico":  {},
}

// Discoverer finds application icons inside extracted AppImage filesystems.
type Discoverer struct{}

// NewDiscoverer creates an icon discoverer.
func NewDiscoverer() Discoverer {
	return Discoverer{}
}

var _ app.IconDiscoverer = Discoverer{}

// Discover finds the best icon under rootDir for iconName.
//
// iconName is usually the Icon value from the desktop entry. It can be an icon
// name without an extension, a relative path, or an absolute path inside the
// extracted AppImage root.
func (Discoverer) Discover(ctx context.Context, rootDir string, iconName string) (app.IconFile, error) {
	if err := ctx.Err(); err != nil {
		return app.IconFile{}, err
	}
	if rootDir == "" {
		return app.IconFile{}, errors.New("icon discovery root directory is required")
	}

	rootDir, err := filepath.Abs(rootDir)
	if err != nil {
		return app.IconFile{}, fmt.Errorf("resolve icon discovery root %q: %w", rootDir, err)
	}

	if iconName != "" {
		if path, ok, err := resolveExplicitIconPath(rootDir, iconName); err != nil {
			return app.IconFile{}, err
		} else if ok {
			return app.IconFile{Path: path}, nil
		}
	}

	candidates, err := iconCandidates(ctx, rootDir, iconName)
	if err != nil {
		return app.IconFile{}, err
	}
	if len(candidates) == 0 {
		return app.IconFile{}, fmt.Errorf("find icon under %q: no supported icon files found", rootDir)
	}

	return app.IconFile{Path: candidates[0]}, nil
}

func resolveExplicitIconPath(rootDir string, iconName string) (string, bool, error) {
	iconName = strings.TrimSpace(iconName)
	if iconName == "" || !filepath.IsAbs(iconName) {
		return "", false, nil
	}

	hostPath, err := filepath.Abs(iconName)
	if err != nil {
		return "", false, fmt.Errorf("resolve icon path %q: %w", iconName, err)
	}

	inside, err := pathInside(rootDir, hostPath)
	if err != nil {
		return "", false, err
	}
	if inside {
		return validateExplicitIconPath(rootDir, hostPath)
	}
	if hasParentDirSegment(iconName) {
		return "", false, fmt.Errorf("icon path %q is outside extracted root %q", iconName, rootDir)
	}

	candidate := filepath.Join(rootDir, strings.TrimPrefix(filepath.Clean(iconName), string(filepath.Separator)))
	candidateInside, err := pathInside(rootDir, candidate)
	if err != nil {
		return "", false, err
	}
	if !candidateInside {
		return "", false, fmt.Errorf("icon path %q is outside extracted root %q", candidate, rootDir)
	}

	path, ok, err := validateExplicitIconPath(rootDir, candidate)
	if err == nil {
		return path, ok, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}

	if _, hostErr := os.Stat(hostPath); hostErr == nil {
		return "", false, fmt.Errorf("icon path %q is outside extracted root %q", hostPath, rootDir)
	}

	return "", false, err
}

func validateExplicitIconPath(rootDir string, path string) (string, bool, error) {
	inside, err := pathInside(rootDir, path)
	if err != nil {
		return "", false, err
	}
	if !inside {
		return "", false, fmt.Errorf("icon path %q is outside extracted root %q", path, rootDir)
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", false, fmt.Errorf("stat icon path %q: %w", path, err)
	}
	if info.IsDir() {
		return "", false, fmt.Errorf("icon path %q is a directory", path)
	}
	if !isSupportedIconPath(path) {
		return "", false, fmt.Errorf("icon path %q has unsupported extension", path)
	}

	return path, true, nil
}

func iconCandidates(ctx context.Context, rootDir string, iconName string) ([]string, error) {
	iconName = strings.TrimSpace(iconName)
	wantedBase := normalizedIconBase(iconName)
	wantedPath := filepath.Clean(iconName)

	var candidates []string
	err := filepath.WalkDir(rootDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if entry.IsDir() {
			return nil
		}
		if !isSupportedIconPath(path) && !isDirIcon(path) {
			return nil
		}
		if iconName != "" && !iconMatches(path, rootDir, wantedBase, wantedPath) {
			return nil
		}

		candidates = append(candidates, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk icon discovery root %q: %w", rootDir, err)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return iconPathScore(rootDir, candidates[i], wantedBase) > iconPathScore(rootDir, candidates[j], wantedBase)
	})

	return candidates, nil
}

func iconMatches(path string, rootDir string, wantedBase string, wantedPath string) bool {
	base := filepath.Base(path)
	if strings.EqualFold(base, filepath.Base(wantedPath)) {
		return true
	}
	if strings.EqualFold(normalizedIconBase(base), wantedBase) {
		return true
	}

	relative, err := filepath.Rel(rootDir, path)
	if err != nil {
		return false
	}

	return strings.EqualFold(filepath.ToSlash(relative), filepath.ToSlash(wantedPath))
}

func iconPathScore(rootDir string, path string, wantedBase string) int {
	relative, err := filepath.Rel(rootDir, path)
	if err != nil {
		relative = path
	}
	relative = filepath.ToSlash(relative)
	base := filepath.Base(path)

	score := 0
	if wantedBase != "" && strings.EqualFold(normalizedIconBase(base), wantedBase) {
		score += 1000
	}
	if isDirIcon(path) {
		score += 900
	}
	if !strings.Contains(relative, "/") {
		score += 400
	}
	if strings.HasPrefix(relative, "usr/share/icons/") {
		score += 300
	}
	if strings.HasPrefix(relative, "usr/share/pixmaps/") {
		score += 250
	}
	if strings.Contains(relative, "/hicolor/") {
		score += 100
	}
	score += iconSizeScore(relative)
	score += iconExtensionScore(path)

	return score
}

func iconSizeScore(path string) int {
	best := 0
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		width, height, ok := strings.Cut(part, "x")
		if !ok || width == "" || height == "" || width != height || !isASCIIInt(width) {
			continue
		}

		size := 0
		for _, r := range width {
			size = size*10 + int(r-'0')
		}
		if size > best {
			best = size
		}
	}

	return best
}

func iconExtensionScore(path string) int {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".svg", ".svgz":
		return 80
	case ".png":
		return 60
	case ".xpm":
		return 40
	case ".ico":
		return 20
	default:
		return 0
	}
}

func normalizedIconBase(value string) string {
	base := filepath.Base(strings.TrimSpace(value))
	ext := filepath.Ext(base)
	if _, ok := supportedIconExtensions[strings.ToLower(ext)]; ok {
		base = strings.TrimSuffix(base, ext)
	}

	return strings.ToLower(base)
}

func isSupportedIconPath(path string) bool {
	_, ok := supportedIconExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}

func isDirIcon(path string) bool {
	return strings.EqualFold(filepath.Base(path), ".DirIcon")
}

func pathInside(rootDir string, path string) (bool, error) {
	relative, err := filepath.Rel(rootDir, path)
	if err != nil {
		return false, fmt.Errorf("compare path %q to root %q: %w", path, rootDir, err)
	}

	return relative == "." || (!strings.HasPrefix(relative, "..") && !filepath.IsAbs(relative)), nil
}

func hasParentDirSegment(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func isASCIIInt(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
