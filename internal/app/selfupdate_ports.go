package app

import "context"

// SelfUpdater installs a requested aim version.
//
// Implementations belong in infrastructure. The app layer decides which version
// should be installed and when user confirmation is required.
type SelfUpdater interface {
	Install(ctx context.Context, version string) error
}
