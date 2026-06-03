package cli

import (
	"context"
	"errors"
	"fmt"
	"github.com/slobbe/appimage-manager/internal/app/clock"
	appupdate "github.com/slobbe/appimage-manager/internal/app/update"
	"github.com/slobbe/appimage-manager/internal/infra/config"
	"github.com/slobbe/appimage-manager/internal/infra/download"
	fsys "github.com/slobbe/appimage-manager/internal/infra/filesystem"
	"net/http"
	"time"
)

var runtimeNowISO = clock.NowISO

type stagedDownloadAdapter struct {
	client func() *http.Client
	nowISO func() string
}

func (adapter stagedDownloadAdapter) AppImageFilename(assetName, downloadURL string) string {
	return download.AppImageFilename(assetName, downloadURL)
}

func (adapter stagedDownloadAdapter) Download(ctx context.Context, assetURL, destination string, onProgress func(appupdate.DownloadProgress)) error {
	return (download.StagedDownloader{
		Client: adapter.client(),
		NowISO: adapter.nowISO,
	}).Download(ctx, assetURL, destination, func(event download.Progress) {
		if onProgress != nil {
			onProgress(appupdate.DownloadProgress{Downloaded: event.Downloaded, Total: event.Total})
		}
	})
}

func (adapter stagedDownloadAdapter) RemoveStaged(downloadPath string) {
	download.RemoveStaged(downloadPath)
}

func (adapter stagedDownloadAdapter) StableDestination(dir, assetURL, nameHint string) (string, error) {
	return download.StableDestination(dir, assetURL, nameHint)
}

type hashVerifierAdapter struct{}

func (hashVerifierAdapter) VerifyHashes(path, expectedSHA256, expectedSHA1 string) error {
	return fsys.VerifyHashes(path, expectedSHA256, expectedSHA1)
}

type runtimeDownloadMetadata struct {
	URL          string
	ETag         string
	LastModified string
	TotalBytes   int64
}

type runtimeDownloadRequest struct {
	URL         string
	Destination string
	Metadata    *runtimeDownloadMetadata
}

type runtimeDownloadProgress struct {
	Downloaded int64
	Total      int64
	Metadata   runtimeDownloadMetadata
}

type runtimeDownloadStatusError struct {
	Status string
	Code   int
}

func (err *runtimeDownloadStatusError) Error() string {
	return fmt.Sprintf("download failed with status %s", err.Status)
}

func setRuntimeDownloadTimeout(timeout time.Duration) {
	download.SetHTTPClientTimeout(timeout)
}

func runtimeDownloadHTTPClient() *http.Client {
	return download.SharedHTTPClient()
}

func runtimeDownload(ctx context.Context, req runtimeDownloadRequest, onProgress func(runtimeDownloadProgress)) (*runtimeDownloadMetadata, error) {
	result, err := (download.Downloader{Client: download.SharedHTTPClient()}).Download(ctx, download.Request{
		URL:         req.URL,
		Destination: req.Destination,
		Metadata:    downloadMetadataFromRuntime(req.Metadata),
	}, func(event download.Progress) {
		if onProgress != nil {
			metadata := runtimeMetadataFromDownload(&event.Metadata)
			onProgress(runtimeDownloadProgress{
				Downloaded: event.Downloaded,
				Total:      event.Total,
				Metadata:   *metadata,
			})
		}
	})
	if err != nil {
		var statusErr *download.StatusError
		if errors.As(err, &statusErr) {
			return nil, &runtimeDownloadStatusError{Status: statusErr.Status, Code: statusErr.Code}
		}
		return nil, err
	}
	return runtimeMetadataFromDownload(result), nil
}

func downloadMetadataFromRuntime(meta *runtimeDownloadMetadata) *download.Metadata {
	if meta == nil {
		return nil
	}
	return &download.Metadata{
		URL:          meta.URL,
		ETag:         meta.ETag,
		LastModified: meta.LastModified,
		TotalBytes:   meta.TotalBytes,
	}
}

func runtimeMetadataFromDownload(meta *download.Metadata) *runtimeDownloadMetadata {
	if meta == nil {
		return nil
	}
	return &runtimeDownloadMetadata{
		URL:          meta.URL,
		ETag:         meta.ETag,
		LastModified: meta.LastModified,
		TotalBytes:   meta.TotalBytes,
	}
}

func runtimeRemoveFileIfExists(path string) error {
	return fsys.RemoveFileIfExists(path)
}

func runtimeTempDir() string {
	return config.TempDir
}

func runtimeHasExtension(src string, ext string) bool {
	return fsys.HasExtension(src, ext)
}
