package appimage

import (
	"context"
	"os"
)

type DesktopEntryCandidate struct {
	Path string
	Stem string
}

type Extraction struct {
	Dir     string
	RootDir string
}

type CleanupFunc func()

type Filesystem interface {
	Copy(src string, dst string) (string, error)
	EnsureDir(path string) error
	HasExtension(src string, ext string) bool
	LocateDesktopEntry(root string) (*DesktopEntryCandidate, error)
	LocateIcon(root string) (string, error)
	MakeAbsolute(path string) (string, error)
	Move(src string, dst string) (string, error)
	ReadTextFile(path string) (string, error)
	RequireRegularFile(path string, subject string) (os.FileInfo, error)
}

type Extractor interface {
	Extract(ctx context.Context, src string, tempBaseDir string) (*Extraction, CleanupFunc, error)
	Inspect(ctx context.Context, src string) (*Extraction, CleanupFunc, error)
}

type DesktopEntryRewriter interface {
	RewriteDesktopEntryFile(path, execPath, iconValue string) error
	SanitizeDesktopStem(value string) string
	DesktopStemFromPath(path string) string
}

var (
	defaultFilesystem           Filesystem
	defaultExtractor            Extractor
	defaultDesktopEntryRewriter DesktopEntryRewriter
)

func SetFilesystem(filesystem Filesystem) {
	defaultFilesystem = filesystem
}

func SetExtractor(extractor Extractor) {
	defaultExtractor = extractor
}

func SetDesktopEntryRewriter(rewriter DesktopEntryRewriter) {
	defaultDesktopEntryRewriter = rewriter
}
