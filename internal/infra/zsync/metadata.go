package zsync

import (
	"fmt"
	"io"
	"net/http"
)

const MetadataMaxBytes = 1 << 20

type Client struct {
	HTTPClient *http.Client
}

func (c Client) FetchMetadata(url string) ([]byte, error) {
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
