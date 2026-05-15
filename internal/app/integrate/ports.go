package integrate

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

var (
	defaultFilesystem          Filesystem
	defaultDesktopLinkResolver DesktopLinkResolver
)

func SetFilesystem(filesystem Filesystem) {
	defaultFilesystem = filesystem
}

func SetDesktopLinkResolver(resolver DesktopLinkResolver) {
	defaultDesktopLinkResolver = resolver
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
