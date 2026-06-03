package integrate

import (
	"context"
	"os"
	"testing"

	appimageapp "github.com/slobbe/appimage-manager/internal/app/appimage"
	appimageinfra "github.com/slobbe/appimage-manager/internal/infra/appimage"
	"github.com/slobbe/appimage-manager/internal/infra/desktop"
	fsys "github.com/slobbe/appimage-manager/internal/infra/filesystem"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

type testAppImageFilesystem struct{}

func (testAppImageFilesystem) Copy(src string, dst string) (string, error) {
	return fsys.Copy(src, dst)
}
func (testAppImageFilesystem) EnsureDir(path string) error { return fsys.EnsureDir(path) }
func (testAppImageFilesystem) HasExtension(src string, ext string) bool {
	return fsys.HasExtension(src, ext)
}
func (testAppImageFilesystem) LocateIcon(root string) (string, error) { return fsys.LocateIcon(root) }
func (testAppImageFilesystem) MakeAbsolute(path string) (string, error) {
	return fsys.MakeAbsolute(path)
}
func (testAppImageFilesystem) Move(src string, dst string) (string, error) {
	return fsys.Move(src, dst)
}
func (testAppImageFilesystem) MakeExecutable(path string) error { return fsys.MakeExecutable(path) }
func (testAppImageFilesystem) RemoveAll(path string) error      { return fsys.RemoveAll(path) }
func (testAppImageFilesystem) RemoveFileIfExists(path string) error {
	return fsys.RemoveFileIfExists(path)
}
func (testAppImageFilesystem) ReplaceSymlink(src string, linkPath string) error {
	return fsys.ReplaceSymlink(src, linkPath)
}
func (testAppImageFilesystem) Sha256AndSha1(path string) (string, string, error) {
	return fsys.Sha256AndSha1(path)
}
func (testAppImageFilesystem) ReadTextFile(path string) (string, error) {
	return fsys.ReadTextFile(path)
}
func (testAppImageFilesystem) RequireRegularFile(path string, subject string) (os.FileInfo, error) {
	return fsys.RequireRegularFile(path, subject)
}
func (testAppImageFilesystem) LocateDesktopEntry(root string) (*appimageapp.DesktopEntryCandidate, error) {
	candidate, err := fsys.LocateDesktopEntry(root)
	if err != nil {
		return nil, err
	}
	return &appimageapp.DesktopEntryCandidate{Path: candidate.Path, Stem: candidate.Stem}, nil
}

type testAppImageExtractor struct{}

func (testAppImageExtractor) Extract(ctx context.Context, src string, tempBaseDir string) (*appimageapp.Extraction, appimageapp.CleanupFunc, error) {
	extraction, cleanup, err := appimageinfra.Extract(ctx, src, tempBaseDir)
	if err != nil {
		return nil, nil, err
	}
	return &appimageapp.Extraction{Dir: extraction.Dir, RootDir: extraction.RootDir}, appimageapp.CleanupFunc(cleanup), nil
}

func (testAppImageExtractor) Inspect(ctx context.Context, src string) (*appimageapp.Extraction, appimageapp.CleanupFunc, error) {
	extraction, cleanup, err := appimageinfra.Inspect(ctx, src)
	if err != nil {
		return nil, nil, err
	}
	return &appimageapp.Extraction{Dir: extraction.Dir, RootDir: extraction.RootDir}, appimageapp.CleanupFunc(cleanup), nil
}

type testAppImageDesktopEntryRewriter struct{}

func (testAppImageDesktopEntryRewriter) RewriteDesktopEntryFile(path, execPath, iconValue string) error {
	return desktop.RewriteDesktopEntryFile(path, execPath, iconValue)
}
func (testAppImageDesktopEntryRewriter) SanitizeDesktopStem(value string) string {
	return desktop.SanitizeDesktopStem(value)
}
func (testAppImageDesktopEntryRewriter) DesktopStemFromPath(path string) string {
	return desktop.DesktopStemFromPath(path)
}

type testDesktopLinkResolver struct{}

func (testDesktopLinkResolver) ResolveDesktopLinkPath(desktopDir, src, preferredName, fallbackName string) (string, error) {
	return desktop.ResolveDesktopLinkPath(desktopDir, src, preferredName, fallbackName)
}
