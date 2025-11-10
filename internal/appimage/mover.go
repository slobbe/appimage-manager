package appimage

import (
	"os"
	"path/filepath"
)

// MoveToLibrary moves AppImage files to the given standard directory (relative to $HOME).
// Returns final absolute paths.
func MoveToLibrary(paths []string, libraryDir string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	libraryPath := filepath.Join(home, libraryDir)
	if err := os.MkdirAll(libraryPath, 0755); err != nil {
		return nil, err
	}

	var moved []string
	for _, src := range paths {
		dest := src
		if filepath.Dir(src) != libraryPath {
			dest = filepath.Join(libraryPath, filepath.Base(src))

			// Avoid overwriting an existing file
			if _, err := os.Stat(dest); err == nil {
				dest = filepath.Join(libraryPath, "_"+filepath.Base(src))
			}

			if err := os.Rename(src, dest); err != nil {
				continue // skip failed moves
			}
		}

		// Make file executable after move
		_ = MakeExecutable(dest)

		moved = append(moved, dest)
	}

	return moved, nil
}
