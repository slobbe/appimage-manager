package upgrade

import "context"

type SelfUpdater interface {
	FetchLatestReleaseTag(ctx context.Context, releaseURL string) (string, error)
	ReadInstalledVersion(ctx context.Context, binaryPath string) (string, error)
	ResolveInstalledPath() (string, error)
	RunInstallerScript(ctx context.Context, scriptURL string, tempDir func() (string, error)) error
}

var defaultSelfUpdater SelfUpdater
