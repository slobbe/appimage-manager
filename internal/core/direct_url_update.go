package core

import (
	"fmt"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/types"
)

type DirectURLUpdate struct {
	Available   bool
	DownloadURL string
	AssetName   string
	SHA256      string
}

func DirectURLUpdateCheck(update *models.UpdateSource, currentSHA256 string) (*DirectURLUpdate, error) {
	if update == nil || update.Kind != models.UpdateDirectURL || update.DirectURL == nil {
		return nil, fmt.Errorf("invalid direct url update source")
	}

	downloadURL := strings.TrimSpace(update.DirectURL.URL)
	if !isHTTPSURL(downloadURL) {
		return nil, fmt.Errorf("direct url must be a valid https URL")
	}

	expectedSHA := strings.ToLower(strings.TrimSpace(update.DirectURL.SHA256))
	if !isValidSHA256Hex(expectedSHA) {
		return nil, fmt.Errorf("direct url sha256 must be a 64-character hex value")
	}

	currentHash := strings.ToLower(strings.TrimSpace(currentSHA256))
	available := currentHash == "" || currentHash != expectedSHA

	return &DirectURLUpdate{
		Available:   available,
		DownloadURL: downloadURL,
		AssetName:   filenameFromURL(downloadURL),
		SHA256:      expectedSHA,
	}, nil
}
