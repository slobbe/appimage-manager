package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/types"
)

type ManifestUpdate struct {
	Available   bool
	DownloadURL string
	AssetName   string
	Version     string
	SHA256      string
}

type manifestPayload struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Assets  map[string]struct {
		URL    string `json:"url"`
		SHA256 string `json:"sha256"`
	} `json:"assets"`
}

var manifestHTTPClient = http.DefaultClient

func ManifestUpdateCheck(update *models.UpdateSource, currentVersion, currentSHA256 string) (*ManifestUpdate, error) {
	if update == nil || update.Kind != models.UpdateManifest || update.Manifest == nil {
		return nil, fmt.Errorf("invalid manifest update source")
	}

	manifestURL := strings.TrimSpace(update.Manifest.URL)
	if !isHTTPSURL(manifestURL) {
		return nil, fmt.Errorf("manifest url must be a valid https URL")
	}

	req, err := http.NewRequest(http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := manifestHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("manifest endpoint returned status %s", resp.Status)
	}

	var payload manifestPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	downloadURL, expectedSHA256, err := resolveManifestAsset(payload)
	if err != nil {
		return nil, err
	}

	assetName := filenameFromURL(downloadURL)
	currentHash := strings.ToLower(strings.TrimSpace(currentSHA256))
	available := currentHash == "" || currentHash != expectedSHA256

	if !available {
		latest := normalizeVersion(payload.Version)
		current := normalizeVersion(currentVersion)
		if latest != "" && current != "" && latest != current {
			available = true
		}
	}

	return &ManifestUpdate{
		Available:   available,
		DownloadURL: downloadURL,
		AssetName:   assetName,
		Version:     strings.TrimSpace(payload.Version),
		SHA256:      expectedSHA256,
	}, nil
}

func resolveManifestAsset(payload manifestPayload) (string, string, error) {
	if len(payload.Assets) == 0 {
		urlValue := strings.TrimSpace(payload.URL)
		shaValue := strings.ToLower(strings.TrimSpace(payload.SHA256))
		if !isHTTPSURL(urlValue) {
			return "", "", fmt.Errorf("manifest url entry must be a valid https URL")
		}
		if !isValidSHA256Hex(shaValue) {
			return "", "", fmt.Errorf("manifest sha256 must be a 64-character hex value")
		}
		return urlValue, shaValue, nil
	}

	for _, key := range archAliases(runtime.GOARCH) {
		asset, ok := payload.Assets[key]
		if !ok {
			continue
		}

		urlValue := strings.TrimSpace(asset.URL)
		shaValue := strings.ToLower(strings.TrimSpace(asset.SHA256))
		if !isHTTPSURL(urlValue) {
			return "", "", fmt.Errorf("manifest asset url for %s must be a valid https URL", key)
		}
		if !isValidSHA256Hex(shaValue) {
			return "", "", fmt.Errorf("manifest asset sha256 for %s must be a 64-character hex value", key)
		}
		return urlValue, shaValue, nil
	}

	return "", "", fmt.Errorf("manifest does not include assets for architecture %s", runtime.GOARCH)
}
