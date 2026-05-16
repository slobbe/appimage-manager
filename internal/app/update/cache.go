package update

import (
	"strings"
	"time"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

const CheckCacheVersion = 1

const DefaultCheckCacheTTL = 5 * time.Minute

type CheckCacheFile struct {
	Version int                        `json:"version"`
	Entries map[string]CheckCacheEntry `json:"entries"`
}

type CheckCacheEntry struct {
	SourceKey string            `json:"source_key"`
	CheckedAt string            `json:"checked_at"`
	Update    *CachedUpdateData `json:"update,omitempty"`
}

type CachedUpdateData struct {
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

func NewCheckCacheFile() *CheckCacheFile {
	return &CheckCacheFile{Version: CheckCacheVersion, Entries: map[string]CheckCacheEntry{}}
}

func NormalizeCheckCache(cache *CheckCacheFile) *CheckCacheFile {
	if cache == nil {
		return NewCheckCacheFile()
	}
	cache.Version = CheckCacheVersion
	if cache.Entries == nil {
		cache.Entries = map[string]CheckCacheEntry{}
	}
	return cache
}

func CachedManagedUpdateForApp(cache *CheckCacheFile, app *models.App, sourceKey string, now time.Time, ttl time.Duration) (*ManagedUpdate, bool) {
	if cache == nil || app == nil {
		return nil, false
	}
	if ttl <= 0 {
		ttl = DefaultCheckCacheTTL
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
	if now.IsZero() {
		now = time.Now()
	}
	if now.Sub(checkedAt) > ttl {
		return nil, false
	}
	return entry.ToManagedUpdate(app), true
}

func (entry CheckCacheEntry) ToManagedUpdate(app *models.App) *ManagedUpdate {
	if entry.Update == nil {
		return nil
	}
	return &ManagedUpdate{
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

func SetCachedManagedUpdate(cache *CheckCacheFile, app *models.App, sourceKey string, update *ManagedUpdate, checkedAt string) {
	if cache == nil || app == nil {
		return
	}
	NormalizeCheckCache(cache)
	cache.Entries[strings.TrimSpace(app.ID)] = CheckCacheEntry{
		SourceKey: strings.TrimSpace(sourceKey),
		CheckedAt: strings.TrimSpace(checkedAt),
		Update:    NewCachedUpdateData(update),
	}
}

func InvalidateCachedManagedUpdates(cache *CheckCacheFile, appIDs ...string) {
	if cache == nil || cache.Entries == nil {
		return
	}
	for _, id := range appIDs {
		delete(cache.Entries, strings.TrimSpace(id))
	}
}

func NewCachedUpdateData(update *ManagedUpdate) *CachedUpdateData {
	if update == nil {
		return nil
	}
	return &CachedUpdateData{
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
