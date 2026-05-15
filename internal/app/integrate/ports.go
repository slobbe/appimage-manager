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

var (
	defaultFilesystem                       Filesystem
	defaultDesktopLinkResolver              DesktopLinkResolver
	defaultDesktopEntryValidator            DesktopEntryValidator
	defaultDesktopIntegrationCacheRefresher DesktopIntegrationCacheRefresher
)

func SetFilesystem(filesystem Filesystem) {
	defaultFilesystem = filesystem
}

func SetDesktopLinkResolver(resolver DesktopLinkResolver) {
	defaultDesktopLinkResolver = resolver
}

func SetDesktopEntryValidator(validator DesktopEntryValidator) {
	defaultDesktopEntryValidator = validator
}

func SetDesktopIntegrationCacheRefresher(refresher DesktopIntegrationCacheRefresher) {
	defaultDesktopIntegrationCacheRefresher = refresher
}

func requireFilesystem() (Filesystem, error) {
	if defaultFilesystem == nil {
		return nil, errNotConfigured("integrate filesystem")
	}
	return defaultFilesystem, nil
}

func requireDesktopLinkResolver() (DesktopLinkResolver, error) {
	if defaultDesktopLinkResolver == nil {
		return nil, errNotConfigured("desktop link resolver")
	}
	return defaultDesktopLinkResolver, nil
}

func requireDesktopEntryValidator() (DesktopEntryValidator, error) {
	if defaultDesktopEntryValidator == nil {
		return nil, errNotConfigured("desktop entry validator")
	}
	return defaultDesktopEntryValidator, nil
}

func requireDesktopIntegrationCacheRefresher() (DesktopIntegrationCacheRefresher, error) {
	if defaultDesktopIntegrationCacheRefresher == nil {
		return nil, errNotConfigured("desktop integration cache refresher")
	}
	return defaultDesktopIntegrationCacheRefresher, nil
}
