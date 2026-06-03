package update

import models "github.com/slobbe/appimage-manager/internal/domain"

type releaseTransportDetails struct {
	Transport    string
	ZsyncURL     string
	ExpectedSHA1 string
}

func resolveReleaseTransport(downloadURL, localSHA1 string) releaseTransportDetails {
	return resolveReleaseTransportWithFetcher(downloadURL, localSHA1, nil)
}

func resolveReleaseTransportWithFetcher(downloadURL, localSHA1 string, fetcher ZsyncMetadataFetcher) releaseTransportDetails {
	transport, err := probeReleaseZsyncTransportWithFetcher(downloadURL, localSHA1, fetcher)
	var probed *models.ReleaseTransport
	if transport != nil {
		probed = &models.ReleaseTransport{
			Transport:    transport.Transport,
			ZsyncURL:     transport.ZsyncURL,
			ExpectedSHA1: transport.ExpectedSHA1,
		}
	}
	selected := models.SelectReleaseTransport(probed, err)
	return releaseTransportDetails{
		Transport:    selected.Transport,
		ZsyncURL:     selected.ZsyncURL,
		ExpectedSHA1: selected.ExpectedSHA1,
	}
}
