package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type Metadata struct {
	URL          string
	ETag         string
	LastModified string
	TotalBytes   int64
}

type Progress struct {
	Downloaded int64
	Total      int64
	Metadata   Metadata
}

type Request struct {
	URL         string
	Destination string
	Metadata    *Metadata
}

type Downloader struct {
	Client *http.Client
}

type StatusError struct {
	Status string
	Code   int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("download failed with status %s", e.Status)
}

func (d Downloader) Download(ctx context.Context, req Request, onProgress func(Progress)) (*Metadata, error) {
	client := d.Client
	if client == nil {
		client = http.DefaultClient
	}

	meta := req.Metadata
	if meta != nil && strings.TrimSpace(meta.URL) != "" && strings.TrimSpace(meta.URL) != strings.TrimSpace(req.URL) {
		meta = nil
	}

	existingSize, err := existingFileSize(req.Destination)
	if err != nil {
		return nil, err
	}
	if req.Metadata != meta {
		existingSize = 0
	}

	resp, err := doRequest(ctx, client, req.URL, existingSize)
	if err != nil {
		return nil, err
	}
	if existingSize > 0 && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		existingSize = 0
		meta = nil
		resp, err = doRequest(ctx, client, req.URL, 0)
		if err != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &StatusError{Status: resp.Status, Code: resp.StatusCode}
	}

	openFlags := os.O_CREATE | os.O_WRONLY
	if existingSize > 0 && resp.StatusCode == http.StatusPartialContent {
		openFlags |= os.O_APPEND
	} else {
		openFlags |= os.O_TRUNC
		existingSize = 0
	}

	file, err := os.OpenFile(req.Destination, openFlags, 0o644)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	total := resp.ContentLength
	if existingSize > 0 && resp.ContentLength > 0 {
		total = existingSize + resp.ContentLength
	}

	if meta == nil {
		meta = &Metadata{}
	}
	meta.URL = req.URL
	meta.ETag = strings.TrimSpace(resp.Header.Get("ETag"))
	meta.LastModified = strings.TrimSpace(resp.Header.Get("Last-Modified"))
	meta.TotalBytes = total

	downloaded := existingSize
	buffer := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if _, err := file.Write(buffer[:n]); err != nil {
				return meta, err
			}
			downloaded += int64(n)
			meta.TotalBytes = total
			if onProgress != nil {
				onProgress(Progress{Downloaded: downloaded, Total: total, Metadata: *meta})
			}
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return meta, readErr
		}
	}

	if onProgress != nil {
		onProgress(Progress{Downloaded: downloaded, Total: total, Metadata: *meta})
	}

	return meta, nil
}

func existingFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return info.Size(), nil
}

func doRequest(ctx context.Context, client *http.Client, url string, rangeStart int64) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if rangeStart > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", rangeStart))
	}
	return client.Do(req)
}
