package appimage

import (
	"context"
	"os"
	"testing"

	appimageinfra "github.com/slobbe/appimage-manager/internal/infra/appimage"
	"github.com/slobbe/appimage-manager/internal/infra/desktop"
	fsys "github.com/slobbe/appimage-manager/internal/infra/filesystem"
)

func TestMain(m *testing.M) {
	SetFilesystem(testFilesystem{})
	SetExtractor(testExtractor{})
	SetDesktopEntryRewriter(testDesktopEntryRewriter{})
	os.Exit(m.Run())
}

type testFilesystem struct{}

func (testFilesystem) Copy(src string, dst string) (string, error) {
	return fsys.Copy(src, dst)
}

func (testFilesystem) EnsureDir(path string) error {
	return fsys.EnsureDir(path)
}

func (testFilesystem) HasExtension(src string, ext string) bool {
	return fsys.HasExtension(src, ext)
}

func (testFilesystem) LocateDesktopEntry(root string) (*DesktopEntryCandidate, error) {
	candidate, err := fsys.LocateDesktopEntry(root)
	if err != nil {
		return nil, err
	}
	return &DesktopEntryCandidate{Path: candidate.Path, Stem: candidate.Stem}, nil
}

func (testFilesystem) LocateIcon(root string) (string, error) {
	return fsys.LocateIcon(root)
}

func (testFilesystem) MakeAbsolute(path string) (string, error) {
	return fsys.MakeAbsolute(path)
}

func (testFilesystem) Move(src string, dst string) (string, error) {
	return fsys.Move(src, dst)
}

func (testFilesystem) ReadTextFile(path string) (string, error) {
	return fsys.ReadTextFile(path)
}

func (testFilesystem) RequireRegularFile(path string, subject string) (os.FileInfo, error) {
	return fsys.RequireRegularFile(path, subject)
}

type testExtractor struct{}

func (testExtractor) Extract(ctx context.Context, src string, tempBaseDir string) (*Extraction, CleanupFunc, error) {
	extraction, cleanup, err := appimageinfra.Extract(ctx, src, tempBaseDir)
	if err != nil {
		return nil, nil, err
	}
	return &Extraction{Dir: extraction.Dir, RootDir: extraction.RootDir}, CleanupFunc(cleanup), nil
}

func (testExtractor) Inspect(ctx context.Context, src string) (*Extraction, CleanupFunc, error) {
	extraction, cleanup, err := appimageinfra.Inspect(ctx, src)
	if err != nil {
		return nil, nil, err
	}
	return &Extraction{Dir: extraction.Dir, RootDir: extraction.RootDir}, CleanupFunc(cleanup), nil
}

type testDesktopEntryRewriter struct{}

func (testDesktopEntryRewriter) RewriteDesktopEntryFile(path, execPath, iconValue string) error {
	return desktop.RewriteDesktopEntryFile(path, execPath, iconValue)
}

func (testDesktopEntryRewriter) SanitizeDesktopStem(value string) string {
	return desktop.SanitizeDesktopStem(value)
}

func (testDesktopEntryRewriter) DesktopStemFromPath(path string) string {
	return desktop.DesktopStemFromPath(path)
}
