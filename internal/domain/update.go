package domain

import (
	"fmt"
	"strings"
)

type UpdateKind string

const (
	UpdateNone          UpdateKind = "none"
	UpdateZsync         UpdateKind = "zsync"
	UpdateGitHubRelease UpdateKind = "github_release"
)

type UpdateSource struct {
	Kind          UpdateKind                 `json:"kind"`
	Zsync         *ZsyncUpdateSource         `json:"zsync,omitempty"`
	GitHubRelease *GitHubReleaseUpdateSource `json:"github_release,omitempty"`
}

type ZsyncUpdateSource struct {
	UpdateInfo string `json:"update_info"`
	Transport  string `json:"transport"` // zsync | gh-releases
}

type GitHubReleaseUpdateSource struct {
	Repo        string `json:"repo"`
	Asset       string `json:"asset"`
	ReleaseKind string `json:"release_kind,omitempty"`
}

type ReleaseUpdateData struct {
	Available         bool
	LatestVersion     string
	DownloadURL       string
	AssetName         string
	PreRelease        bool
	Transport         string
	ZsyncURL          string
	ExpectedSHA1      string
	AvailabilityLabel string
}

type ReleaseTransport struct {
	Transport    string
	ZsyncURL     string
	ExpectedSHA1 string
}

func ValidateUpdateSource(source *UpdateSource) error {
	if source == nil {
		return fmt.Errorf("missing update source")
	}

	switch source.Kind {
	case UpdateNone:
		return nil
	case UpdateZsync:
		if source.Zsync == nil || strings.TrimSpace(source.Zsync.UpdateInfo) == "" {
			return fmt.Errorf("missing zsync update info")
		}
		return nil
	case UpdateGitHubRelease:
		if source.GitHubRelease == nil {
			return fmt.Errorf("missing github release update source")
		}
		if strings.TrimSpace(source.GitHubRelease.Repo) == "" {
			return fmt.Errorf("missing github release repo")
		}
		if strings.TrimSpace(source.GitHubRelease.Asset) == "" {
			return fmt.Errorf("missing github release asset")
		}
		return nil
	default:
		return fmt.Errorf("unsupported update source %q", source.Kind)
	}
}

func NewReleaseUpdate(currentVersion, tagName, downloadURL, assetName string, preRelease bool, transport ReleaseTransport) ReleaseUpdateData {
	latest, available := ReleaseAvailability(currentVersion, tagName)
	label := ""
	if available {
		label = UpdateAvailabilityLabel(preRelease)
	}

	return ReleaseUpdateData{
		Available:         available,
		LatestVersion:     latest,
		DownloadURL:       strings.TrimSpace(downloadURL),
		AssetName:         strings.TrimSpace(assetName),
		PreRelease:        preRelease,
		Transport:         strings.TrimSpace(transport.Transport),
		ZsyncURL:          strings.TrimSpace(transport.ZsyncURL),
		ExpectedSHA1:      strings.TrimSpace(transport.ExpectedSHA1),
		AvailabilityLabel: label,
	}
}

func UpdateAvailabilityLabel(preRelease bool) string {
	if preRelease {
		return "Pre-release update available"
	}
	return "Update available"
}

func SelectReleaseTransport(probed *ReleaseTransport, probeErr error) ReleaseTransport {
	if probeErr != nil || probed == nil {
		return ReleaseTransport{}
	}
	return ReleaseTransport{
		Transport:    strings.TrimSpace(probed.Transport),
		ZsyncURL:     strings.TrimSpace(probed.ZsyncURL),
		ExpectedSHA1: strings.TrimSpace(probed.ExpectedSHA1),
	}
}
