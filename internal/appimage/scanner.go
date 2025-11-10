package appimage

import (
	"os"
	"path/filepath"
)

// Scan searches for .AppImage files inside given directories (relative to $HOME)
// and returns their absolute paths.
func Scan(dirs []string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	var results []string

	var isAppImage = func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && filepath.Ext(d.Name()) == ".AppImage" {
			results = append(results, path)
		}
		return nil
	}

	for _, dir := range dirs {
		path := filepath.Join(home, dir)
		err = filepath.WalkDir(path, isAppImage)
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}
