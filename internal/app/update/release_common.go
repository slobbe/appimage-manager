package update

type releaseTransportDetails struct {
	Transport    string
	ZsyncURL     string
	ExpectedSHA1 string
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
