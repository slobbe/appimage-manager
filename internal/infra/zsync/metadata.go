package zsync

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/slobbe/appimage-manager/internal/domain"
)

const MetadataMaxBytes = 1 << 20

type Client struct {
	HTTPClient *http.Client
}

func (c Client) FetchMetadataBytes(url string) ([]byte, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("zsync metadata returned status %s", resp.Status)
	}

	metadata, err := io.ReadAll(io.LimitReader(resp.Body, MetadataMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read zsync metadata: %w", err)
	}
	if len(metadata) > MetadataMaxBytes {
		return nil, fmt.Errorf("zsync metadata exceeds %d bytes", MetadataMaxBytes)
	}

	return metadata, nil
}

func (c Client) FetchMetadata(url string) (*domain.ZsyncMetadata, error) {
	metadata, err := c.FetchMetadataBytes(url)
	if err != nil {
		return nil, err
	}
	return ParseMetadata(metadata)
}

func ParseMetadata(metadata []byte) (*domain.ZsyncMetadata, error) {
	update := &domain.ZsyncMetadata{}

	scanner := bufio.NewScanner(bytes.NewReader(metadata))
	scanner.Buffer(make([]byte, 64*1024), MetadataMaxBytes+1)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}

		sha1, exists := strings.CutPrefix(line, "SHA-1:")
		if exists {
			update.RemoteSHA1 = strings.TrimSpace(sha1)
		}

		filename, exists := strings.CutPrefix(line, "Filename:")
		if exists {
			update.RemoteFilename = strings.TrimSpace(filename)
		}

		mtime, exists := strings.CutPrefix(line, "MTime:")
		if exists {
			t, _ := time.Parse(time.RFC1123, strings.TrimSpace(mtime))
			update.RemoteTime = t.Format(time.RFC3339)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read zsync metadata: %w", err)
	}

	if update.RemoteFilename != "" {
		update.RemoteFilename = strings.TrimSuffix(update.RemoteFilename, ".zsync")
	}
	if update.RemoteFilename == "" || update.RemoteSHA1 == "" {
		return nil, fmt.Errorf("invalid zsync metadata")
	}

	return update, nil
}
