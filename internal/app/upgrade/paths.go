package upgrade

import "fmt"

type Paths struct {
	TempDir string
}

var defaultPaths Paths

func SetPaths(paths Paths) {
	defaultPaths = paths
}

func requirePaths() (Paths, error) {
	if defaultPaths.TempDir == "" {
		return Paths{}, fmt.Errorf("upgrade paths are not configured")
	}
	return defaultPaths, nil
}
