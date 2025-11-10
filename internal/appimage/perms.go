package appimage

import "os"

// IsExecutable returns true if any execute bit is set (user, group, or others).
func IsExecutable(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Mode().Perm()&0111 != 0, nil
}

// MakeExecutable sets +x if not already executable.
func MakeExecutable(path string) error {
	ok, err := IsExecutable(path)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	return os.Chmod(path, 0755)
}
