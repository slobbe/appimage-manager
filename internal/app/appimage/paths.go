package appimage

import "fmt"

type Paths struct {
	AimDir  string
	TempDir string
}

var defaultPaths Paths

func SetPaths(paths Paths) {
	defaultPaths = paths
}

func requirePaths() (Paths, error) {
	if defaultPaths.AimDir == "" || defaultPaths.TempDir == "" {
		return Paths{}, fmt.Errorf("appimage paths are not configured")
	}
	return defaultPaths, nil
}
