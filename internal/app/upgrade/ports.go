package upgrade

import (
	"context"
	"fmt"
)

type SelfUpdater interface {
	FetchLatestReleaseTag(ctx context.Context, releaseURL string) (string, error)
	ReadInstalledVersion(ctx context.Context, binaryPath string) (string, error)
	ResolveInstalledPath() (string, error)
	RunInstallerScript(ctx context.Context, scriptURL string, tempDir func() (string, error)) error
}

var defaultSelfUpdater SelfUpdater

func SetSelfUpdater(updater SelfUpdater) {
	defaultSelfUpdater = updater
}

func requireSelfUpdater() (SelfUpdater, error) {
	if defaultSelfUpdater == nil {
		return nil, fmt.Errorf("self updater is not configured")
	}
	return defaultSelfUpdater, nil
}
