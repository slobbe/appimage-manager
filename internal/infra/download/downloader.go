package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/slobbe/appimage-manager/internal/app"
)

const (
	maxRequestAttempts = 3
	retryBaseDelay     = 100 * time.Millisecond
)

var defaultHTTPClient = &http.Client{
	Timeout: 30 * time.Minute,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
}

// Downloader downloads remote assets to local files.
type Downloader struct {
	HTTPClient *http.Client
}

func NewDownloader(httpClient *http.Client) Downloader {
	return Downloader{HTTPClient: httpClient}
}

var _ app.AssetDownloader = Downloader{}

func (d Downloader) Download(ctx context.Context, source app.DownloadSource, destinationPath string, progress app.DownloadProgress) (app.DownloadedFile, error) {
	if err := ctx.Err(); err != nil {
		return app.DownloadedFile{}, err
	}
	if strings.TrimSpace(source.URL) == "" {
		return app.DownloadedFile{}, errors.New("download url is required")
	}
	if strings.TrimSpace(destinationPath) == "" {
		return app.DownloadedFile{}, errors.New("download destination path is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return app.DownloadedFile{}, fmt.Errorf("create download request %q: %w", source.URL, err)
	}
	req.Header.Set("User-Agent", "aim")

	resp, err := d.do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return app.DownloadedFile{}, ctxErr
		}
		return app.DownloadedFile{}, fmt.Errorf("download %q: %w", source.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return app.DownloadedFile{}, fmt.Errorf("download %q: server returned %s", source.URL, resp.Status)
	}
	if source.SizeBytes > 0 && resp.ContentLength >= 0 && resp.ContentLength != source.SizeBytes {
		return app.DownloadedFile{}, fmt.Errorf("download %q: size mismatch: expected %d bytes, server reported %d bytes", source.URL, source.SizeBytes, resp.ContentLength)
	}

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return app.DownloadedFile{}, fmt.Errorf("create download directory %q: %w", filepath.Dir(destinationPath), err)
	}

	temporaryPath := destinationPath + ".tmp"
	destination, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return app.DownloadedFile{}, fmt.Errorf("create temporary download file %q: %w", temporaryPath, err)
	}

	reader := io.Reader(resp.Body)
	if source.SizeBytes > 0 {
		reader = io.LimitReader(resp.Body, source.SizeBytes+1)
	}

	written, copyErr := copyWithProgress(ctx, destination, reader, progress)
	closeErr := destination.Close()
	if copyErr != nil {
		_ = os.Remove(temporaryPath)
		return app.DownloadedFile{}, fmt.Errorf("write download %q: %w", temporaryPath, copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(temporaryPath)
		return app.DownloadedFile{}, fmt.Errorf("close download %q: %w", temporaryPath, closeErr)
	}
	if source.SizeBytes > 0 && written != source.SizeBytes {
		_ = os.Remove(temporaryPath)
		return app.DownloadedFile{}, fmt.Errorf("download %q: size mismatch: expected %d bytes, wrote %d bytes", source.URL, source.SizeBytes, written)
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(temporaryPath)
		return app.DownloadedFile{}, err
	}
	if err := os.Rename(temporaryPath, destinationPath); err != nil {
		_ = os.Remove(temporaryPath)
		return app.DownloadedFile{}, fmt.Errorf("replace download %q: %w", destinationPath, err)
	}

	return app.DownloadedFile{Path: destinationPath, SizeBytes: written}, nil
}

func (d Downloader) httpClient() *http.Client {
	if d.HTTPClient != nil {
		return d.HTTPClient
	}
	return defaultHTTPClient
}

func (d Downloader) do(req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < maxRequestAttempts; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(retryBaseDelay * time.Duration(1<<(attempt-1)))
			select {
			case <-req.Context().Done():
				timer.Stop()
				return nil, req.Context().Err()
			case <-timer.C:
			}
		}

		resp, err := d.httpClient().Do(req.Clone(req.Context()))
		if err == nil {
			if !isTransientStatus(resp.StatusCode) || attempt == maxRequestAttempts-1 {
				return resp, nil
			}
			resp.Body.Close()
			continue
		}
		if resp != nil {
			resp.Body.Close()
		}
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

func isTransientStatus(status int) bool {
	return status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout
}

func copyWithProgress(ctx context.Context, dst io.Writer, src io.Reader, progress app.DownloadProgress) (int64, error) {
	buffer := make([]byte, 32*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}

		n, readErr := src.Read(buffer)
		if n > 0 {
			writeN, writeErr := dst.Write(buffer[:n])
			written += int64(writeN)
			if progress != nil && writeN > 0 {
				progress.Advance(int64(writeN))
			}
			if writeErr != nil {
				return written, writeErr
			}
			if writeN != n {
				return written, io.ErrShortWrite
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return written, nil
			}
			return written, readErr
		}
	}
}
