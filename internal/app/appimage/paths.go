package appimage

import "fmt"

type Paths struct {
	AimDir  string
	TempDir string
}

func requirePaths(paths Paths) (Paths, error) {
	if paths.AimDir == "" || paths.TempDir == "" {
		return Paths{}, fmt.Errorf("appimage paths are not configured")
	}
	return paths, nil
}
