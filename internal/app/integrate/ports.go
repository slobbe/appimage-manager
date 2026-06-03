package integrate

import "context"

type Filesystem interface {
	EnsureDir(path string) error
	HasExtension(src string, ext string) bool
	MakeAbsolute(path string) (string, error)
	MakeExecutable(path string) error
	Move(src string, dst string) (string, error)
	RemoveAll(path string) error
	RemoveFileIfExists(path string) error
	ReplaceSymlink(src string, linkPath string) error
	Sha256AndSha1(path string) (string, string, error)
}

type DesktopLinkResolver interface {
	ResolveDesktopLinkPath(desktopDir, src, preferredName, fallbackName string) (string, error)
}

type DesktopEntryValidator interface {
	ValidateDesktopEntry(ctx context.Context, desktopPath string) error
}

type DesktopIntegrationCacheRefresher interface {
	RefreshDesktopIntegrationCaches(ctx context.Context, desktopDir, iconThemeDir string)
}
