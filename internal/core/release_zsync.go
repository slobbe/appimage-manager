package core

import (
	"fmt"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/types"
)

type releaseZsyncTransport struct {
	Transport    string
	ZsyncURL     string
	ExpectedSHA1 string
}

func probeReleaseZsyncTransport(assetURL, localSHA1 string) (*releaseZsyncTransport, error) {
	assetURL = strings.TrimSpace(assetURL)
	if assetURL == "" {
		return nil, fmt.Errorf("missing asset url")
	}

	zsyncURL := assetURL + ".zsync"
	update, err := ZsyncUpdateCheck(&models.UpdateSource{
		Kind: models.UpdateZsync,
		Zsync: &models.ZsyncUpdateSource{
			UpdateInfo: "zsync|" + zsyncURL,
			Transport:  "zsync",
		},
	}, localSHA1)
	if err != nil {
		return nil, err
	}
	if update == nil {
		return nil, fmt.Errorf("missing zsync metadata")
	}

	expectedSHA1 := strings.TrimSpace(update.RemoteSHA1)
	if expectedSHA1 == "" {
		return nil, fmt.Errorf("missing zsync remote sha1")
	}

	return &releaseZsyncTransport{
		Transport:    "zsync",
		ZsyncURL:     zsyncURL,
		ExpectedSHA1: expectedSHA1,
	}, nil
}
