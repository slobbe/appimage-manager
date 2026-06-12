package app

import "context"

// AppImageExtractor extracts AppImages into a workspace directory.
type AppImageExtractor interface {
	Extract(ctx context.Context, appImagePath string, destDir string) (AppImageExtraction, error)
}

// AppImageExtraction describes an extracted AppImage filesystem.
type AppImageExtraction struct {
	RootDir    string
	UpdateInfo string
}

// DesktopEntryDiscoverer finds desktop entry files in extracted AppImages.
type DesktopEntryDiscoverer interface {
	Discover(ctx context.Context, rootDir string) (DesktopEntryFile, error)
}

// DesktopEntryFile is a discovered .desktop file and its contents.
type DesktopEntryFile struct {
	Path    string
	Content []byte
}

// IconDiscoverer finds application icon files in extracted AppImages.
type IconDiscoverer interface {
	Discover(ctx context.Context, rootDir string, iconName string) (IconFile, error)
}

// IconFile is a discovered icon file.
type IconFile struct {
	Path string
}

// AppImageInstaller installs an AppImage into the app library.
type AppImageInstaller interface {
	Install(ctx context.Context, sourcePath string, appID string) (string, error)
}

// AppImageRemover removes an installed AppImage artifact.
type AppImageRemover interface {
	Remove(ctx context.Context, path string) error
}

// IconInstaller installs an icon into the icon directory.
type IconInstaller interface {
	Install(ctx context.Context, sourcePath string, appID string) (string, error)
}

// IconRemover removes an installed icon artifact.
type IconRemover interface {
	Remove(ctx context.Context, path string) error
}

// DesktopEntryInstaller installs a desktop entry into the applications directory.
type DesktopEntryInstaller interface {
	Install(ctx context.Context, appID string, content []byte) (string, error)
}

// DesktopEntryRemover removes an installed desktop entry artifact.
type DesktopEntryRemover interface {
	Remove(ctx context.Context, path string) error
}
