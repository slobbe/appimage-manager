package update

import models "github.com/slobbe/appimage-manager/internal/domain"

type releaseTransportDetails struct {
	Transport    string
	ZsyncURL     string
	ExpectedSHA1 string
}

func normalizeVersion(version string) string {
	return models.NormalizeComparableVersion(version)
}

func releaseAvailability(currentVersion, tagName string) (string, bool) {
	latest := normalizeVersion(tagName)
	current := normalizeVersion(currentVersion)
	return latest, latest != "" && latest != current
}

func resolveReleaseTransport(downloadURL, localSHA1 string) releaseTransportDetails {
	transport, err := probeReleaseZsyncTransport(downloadURL, localSHA1)
	if err != nil || transport == nil {
		return releaseTransportDetails{}
	}
	return releaseTransportDetails{
		Transport:    transport.Transport,
		ZsyncURL:     transport.ZsyncURL,
		ExpectedSHA1: transport.ExpectedSHA1,
	}
}
