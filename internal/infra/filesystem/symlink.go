package filesystem

import (
	"os"
	"path/filepath"
)

func ReplaceSymlink(src string, linkPath string) error {
	_ = os.Remove(linkPath)
	if err := EnsureDir(filepath.Dir(linkPath)); err != nil {
		return err
	}
	return os.Symlink(src, linkPath)
}
