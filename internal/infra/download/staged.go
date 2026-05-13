package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	fsys "github.com/slobbe/appimage-manager/internal/infra/filesystem"
)

type StagedMetadata struct {
	URL          string `json:"url"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	TotalBytes   int64  `json:"total_bytes,omitempty"`
	UpdatedAt    string `json:"updated_at"`
}

type StagedDownloader struct {
	Client *http.Client
	NowISO func() string
}

func StableDestination(dir, assetURL, nameHint string) (string, error) {
	if err := fsys.EnsureDir(dir); err != nil {
		return "", err
	}

	key := strings.TrimSpace(assetURL) + "|" + strings.TrimSpace(nameHint)
	sum := sha256.Sum256([]byte(key))
	fileName := AppImageFilename(nameHint, assetURL)
	base := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	stagedName := fmt.Sprintf("%s-%s%s", base, hex.EncodeToString(sum[:8]), filepath.Ext(fileName))
	return filepath.Join(dir, stagedName), nil
}

func AppImageFilename(assetName, downloadURL string) string {
	name := strings.TrimSpace(filepath.Base(assetName))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = strings.TrimSpace(filepath.Base(downloadURL))
	}
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "update.AppImage"
	}
	if !fsys.HasExtension(name, ".AppImage") {
		name = name + ".AppImage"
	}
	return name
}

func (d StagedDownloader) Download(ctx context.Context, assetURL, destination string, onProgress func(Progress)) error {
	stagedMeta, err := LoadStagedMetadata(destination)
	if err != nil {
		return err
	}

	meta := metadataFromStaged(stagedMeta)
	if meta != nil && strings.TrimSpace(meta.URL) != "" && strings.TrimSpace(meta.URL) != strings.TrimSpace(assetURL) {
		RemoveStaged(destination)
		meta = nil
	}

	resultMeta, err := (Downloader{Client: d.Client}).Download(ctx, Request{
		URL:         assetURL,
		Destination: destination,
		Metadata:    meta,
	}, func(event Progress) {
		if onProgress != nil {
			onProgress(event)
		}
		_ = d.SaveStagedMetadata(destination, stagedFromMetadata(&event.Metadata))
	})
	if resultMeta != nil {
		if err := d.SaveStagedMetadata(destination, stagedFromMetadata(resultMeta)); err != nil {
			return err
		}
	}
	if err != nil {
		var pathErr *os.PathError
		if errors.As(err, &pathErr) {
			return pathErr
		}
		return err
	}

	return nil
}

func LoadStagedMetadata(downloadPath string) (*StagedMetadata, error) {
	data, ok, err := fsys.ReadFileIfExists(StagedMetadataPath(downloadPath))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	var meta StagedMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func (d StagedDownloader) SaveStagedMetadata(downloadPath string, meta StagedMetadata) error {
	if d.NowISO != nil {
		meta.UpdatedAt = d.NowISO()
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return fsys.WriteAtomicFile(StagedMetadataPath(downloadPath), data, 0o644)
}

func StagedMetadataPath(downloadPath string) string {
	return downloadPath + ".meta.json"
}

func RemoveStaged(downloadPath string) {
	_ = fsys.RemoveFileIfExists(downloadPath)
	_ = fsys.RemoveFileIfExists(StagedMetadataPath(downloadPath))
}

func metadataFromStaged(meta *StagedMetadata) *Metadata {
	if meta == nil {
		return nil
	}
	return &Metadata{
		URL:          meta.URL,
		ETag:         meta.ETag,
		LastModified: meta.LastModified,
		TotalBytes:   meta.TotalBytes,
	}
}

func stagedFromMetadata(meta *Metadata) StagedMetadata {
	if meta == nil {
		return StagedMetadata{}
	}
	return StagedMetadata{
		URL:          meta.URL,
		ETag:         meta.ETag,
		LastModified: meta.LastModified,
		TotalBytes:   meta.TotalBytes,
	}
}
