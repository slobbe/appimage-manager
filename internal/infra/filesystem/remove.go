package filesystem

import "os"

func RemoveFileIfExists(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func RemoveAll(path string) error {
	if path == "" {
		return nil
	}
	return os.RemoveAll(path)
}
