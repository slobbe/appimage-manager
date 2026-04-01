package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/slobbe/appimage-manager/internal/config"
	util "github.com/slobbe/appimage-manager/internal/helpers"
	models "github.com/slobbe/appimage-manager/internal/types"
)

const (
	updateCheckCacheVersion = 1
	updateCheckCacheTTL     = 5 * time.Minute
)

type stagedDownloadMetadata struct {
	URL          string `json:"url"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	TotalBytes   int64  `json:"total_bytes,omitempty"`
	UpdatedAt    string `json:"updated_at"`
}

type updateCheckCacheFile struct {
	Version int                              `json:"version"`
	Entries map[string]updateCheckCacheEntry `json:"entries"`
}

type updateCheckCacheEntry struct {
	SourceKey string                   `json:"source_key"`
	CheckedAt string                   `json:"checked_at"`
	Update    *cachedPendingUpdateData `json:"update,omitempty"`
}

type cachedPendingUpdateData struct {
	URL            string            `json:"url,omitempty"`
	Asset          string            `json:"asset,omitempty"`
	Label          string            `json:"label,omitempty"`
	Available      bool              `json:"available"`
	Latest         string            `json:"latest,omitempty"`
	ExpectedSHA1   string            `json:"expected_sha1,omitempty"`
	ExpectedSHA256 string            `json:"expected_sha256,omitempty"`
	Transport      string            `json:"transport,omitempty"`
	ZsyncURL       string            `json:"zsync_url,omitempty"`
	FromKind       models.UpdateKind `json:"source_kind,omitempty"`
}

func stagedDownloadDir() string {
	return filepath.Join(config.TempDir, "downloads")
}

func updateCheckCacheFilePath() string {
	return filepath.Join(config.TempDir, "update-check-cache.json")
}

func stableDownloadDestination(assetURL, nameHint string) (string, error) {
	if err := os.MkdirAll(stagedDownloadDir(), 0o755); err != nil {
		return "", wrapWriteError(err)
	}

	key := strings.TrimSpace(assetURL) + "|" + strings.TrimSpace(nameHint)
	sum := sha256.Sum256([]byte(key))
	fileName := updateDownloadFilename(nameHint, assetURL)
	base := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	stagedName := fmt.Sprintf("%s-%s%s", base, hex.EncodeToString(sum[:8]), filepath.Ext(fileName))
	return filepath.Join(stagedDownloadDir(), stagedName), nil
}

func stagedDownloadMetadataPath(downloadPath string) string {
	return downloadPath + ".meta.json"
}

func loadStagedDownloadMetadata(downloadPath string) (*stagedDownloadMetadata, error) {
	data, err := os.ReadFile(stagedDownloadMetadataPath(downloadPath))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var meta stagedDownloadMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func saveStagedDownloadMetadata(downloadPath string, meta stagedDownloadMetadata) error {
	meta.UpdatedAt = util.NowISO()
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicFile(stagedDownloadMetadataPath(downloadPath), data, 0o644)
}

func removeStagedDownload(downloadPath string) {
	_ = os.Remove(downloadPath)
	_ = os.Remove(stagedDownloadMetadataPath(downloadPath))
}

func loadUpdateCheckCache() (*updateCheckCacheFile, error) {
	path := updateCheckCacheFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &updateCheckCacheFile{
				Version: updateCheckCacheVersion,
				Entries: map[string]updateCheckCacheEntry{},
			}, nil
		}
		return nil, err
	}

	var cache updateCheckCacheFile
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	if cache.Version != updateCheckCacheVersion || cache.Entries == nil {
		cache.Version = updateCheckCacheVersion
		if cache.Entries == nil {
			cache.Entries = map[string]updateCheckCacheEntry{}
		}
	}
	return &cache, nil
}

func saveUpdateCheckCache(cache *updateCheckCacheFile) error {
	if cache == nil {
		return nil
	}
	if err := os.MkdirAll(config.TempDir, 0o755); err != nil {
		return err
	}
	cache.Version = updateCheckCacheVersion
	if cache.Entries == nil {
		cache.Entries = map[string]updateCheckCacheEntry{}
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomicFile(updateCheckCacheFilePath(), data, 0o644)
}

func cachedManagedUpdateForApp(cache *updateCheckCacheFile, app *models.App, sourceKey string) (*pendingManagedUpdate, bool) {
	if cache == nil || app == nil {
		return nil, false
	}
	entry, ok := cache.Entries[strings.TrimSpace(app.ID)]
	if !ok {
		return nil, false
	}
	if strings.TrimSpace(entry.SourceKey) != strings.TrimSpace(sourceKey) {
		return nil, false
	}
	checkedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(entry.CheckedAt))
	if err != nil {
		return nil, false
	}
	if time.Since(checkedAt) > updateCheckCacheTTL {
		return nil, false
	}
	return entry.toPending(app), true
}

func (entry updateCheckCacheEntry) toPending(app *models.App) *pendingManagedUpdate {
	if entry.Update == nil {
		return nil
	}
	return &pendingManagedUpdate{
		App:            app,
		URL:            entry.Update.URL,
		Asset:          entry.Update.Asset,
		Label:          entry.Update.Label,
		Available:      entry.Update.Available,
		Latest:         entry.Update.Latest,
		ExpectedSHA1:   entry.Update.ExpectedSHA1,
		ExpectedSHA256: entry.Update.ExpectedSHA256,
		Transport:      entry.Update.Transport,
		ZsyncURL:       entry.Update.ZsyncURL,
		FromKind:       entry.Update.FromKind,
	}
}

func setCachedManagedUpdate(cache *updateCheckCacheFile, app *models.App, sourceKey string, update *pendingManagedUpdate) {
	if cache == nil || app == nil {
		return
	}
	if cache.Entries == nil {
		cache.Entries = map[string]updateCheckCacheEntry{}
	}
	cache.Entries[strings.TrimSpace(app.ID)] = updateCheckCacheEntry{
		SourceKey: strings.TrimSpace(sourceKey),
		CheckedAt: util.NowISO(),
		Update:    newCachedPendingUpdate(update),
	}
}

func invalidateCachedManagedUpdates(cache *updateCheckCacheFile, appIDs ...string) {
	if cache == nil || cache.Entries == nil {
		return
	}
	for _, id := range appIDs {
		delete(cache.Entries, strings.TrimSpace(id))
	}
}

func newCachedPendingUpdate(update *pendingManagedUpdate) *cachedPendingUpdateData {
	if update == nil {
		return nil
	}
	return &cachedPendingUpdateData{
		URL:            update.URL,
		Asset:          update.Asset,
		Label:          update.Label,
		Available:      update.Available,
		Latest:         update.Latest,
		ExpectedSHA1:   update.ExpectedSHA1,
		ExpectedSHA256: update.ExpectedSHA256,
		Transport:      update.Transport,
		ZsyncURL:       update.ZsyncURL,
		FromKind:       update.FromKind,
	}
}

func appliedAppIDs(apps []*models.App) []string {
	ids := make([]string, 0, len(apps))
	for _, app := range apps {
		if app == nil || strings.TrimSpace(app.ID) == "" {
			continue
		}
		ids = append(ids, strings.TrimSpace(app.ID))
	}
	return ids
}

func writeAtomicFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, perm); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}
