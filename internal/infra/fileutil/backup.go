package fileutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/slobbe/appimage-manager/internal/app"
)

type BackupManager struct{}

var _ app.ArtifactBackupManager = BackupManager{}

type backupEntry struct {
	original string
	snapshot string
}

type artifactBackup struct {
	dir     string
	entries []backupEntry
}

func (BackupManager) Existing(ctx context.Context, paths []string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var existing []string
	for _, path := range uniquePaths(paths) {
		if _, err := os.Lstat(path); err == nil {
			existing = append(existing, path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("inspect artifact %q: %w", path, err)
		}
	}
	return existing, nil
}

func (BackupManager) Backup(ctx context.Context, paths []string) (app.ArtifactBackup, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", "aim-artifact-backup-*")
	if err != nil {
		return nil, fmt.Errorf("create artifact backup: %w", err)
	}
	backup := &artifactBackup{dir: dir}
	for index, path := range uniquePaths(paths) {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			_ = backup.Close()
			return nil, fmt.Errorf("inspect artifact %q for backup: %w", path, err)
		}
		snapshot := filepath.Join(dir, fmt.Sprintf("%d", index))
		if err := CopyFile(ctx, path, snapshot); err != nil {
			_ = backup.Close()
			return nil, fmt.Errorf("back up artifact %q: %w", path, err)
		}
		backup.entries = append(backup.entries, backupEntry{original: path, snapshot: snapshot})
	}
	return backup, nil
}

func uniquePaths(input []string) []string {
	seen := make(map[string]struct{}, len(input))
	paths := make([]string, 0, len(input))
	for _, path := range input {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

func (b *artifactBackup) Restore(ctx context.Context) error {
	var failures []error
	for _, entry := range b.entries {
		if err := CopyFile(ctx, entry.snapshot, entry.original); err != nil {
			failures = append(failures, fmt.Errorf("restore artifact %q: %w", entry.original, err))
		}
	}
	return errors.Join(failures...)
}

func (b *artifactBackup) Close() error {
	if b.dir == "" {
		return nil
	}
	err := os.RemoveAll(b.dir)
	b.dir = ""
	return err
}

func (b *artifactBackup) Location() string {
	return b.dir
}
