package desktop

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

// Discoverer finds desktop entries inside extracted AppImage filesystems.
type Discoverer struct{}

var _ app.DesktopEntryDiscoverer = Discoverer{}

// Discover finds the most likely desktop entry under rootDir and reads it.
func (Discoverer) Discover(ctx context.Context, rootDir string) (app.DesktopEntryFile, error) {
	if err := ctx.Err(); err != nil {
		return app.DesktopEntryFile{}, err
	}
	if rootDir == "" {
		return app.DesktopEntryFile{}, errors.New("desktop discovery root directory is required")
	}

	candidates, err := desktopEntryCandidates(ctx, rootDir)
	if err != nil {
		return app.DesktopEntryFile{}, err
	}
	if len(candidates) == 0 {
		return app.DesktopEntryFile{}, fmt.Errorf("find desktop entry under %q: no .desktop files found", rootDir)
	}

	path := candidates[0]
	content, err := os.ReadFile(path)
	if err != nil {
		return app.DesktopEntryFile{}, fmt.Errorf("read desktop entry %q: %w", path, err)
	}

	return app.DesktopEntryFile{Path: path, Content: content}, nil
}

func desktopEntryCandidates(ctx context.Context, rootDir string) ([]string, error) {
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
		if strings.EqualFold(filepath.Ext(entry.Name()), ".desktop") {
			candidates = append(candidates, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk desktop discovery root %q: %w", rootDir, err)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return desktopEntryPathScore(rootDir, candidates[i]) > desktopEntryPathScore(rootDir, candidates[j])
	})

	return candidates, nil
}

func desktopEntryPathScore(rootDir string, path string) int {
	relative, err := filepath.Rel(rootDir, path)
	if err != nil {
		relative = path
	}
	relative = filepath.ToSlash(relative)

	score := 0
	if !strings.Contains(relative, "/") {
		score += 100
	}
	if strings.HasPrefix(relative, "usr/share/applications/") {
		score += 50
	}
	if strings.Contains(relative, "/applications/") {
		score += 25
	}
	if strings.EqualFold(filepath.Base(path), "app.desktop") {
		score += 10
	}

	return score
}
