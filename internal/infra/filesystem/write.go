package filesystem

import (
	"os"
	"path/filepath"
	"unicode/utf8"
)

func ReadTextFile(path string) (string, error) {
	if _, err := RequireRegularFile(path, "file"); err != nil {
		return "", err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	if !utf8.Valid(content) {
		return "", errInvalidUTF8()
	}

	return string(content), nil
}

func ReadFileIfExists(path string) ([]byte, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

func WriteAtomicFile(path string, data []byte, perm os.FileMode) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, perm); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func errInvalidUTF8() error {
	return &invalidUTF8Error{}
}

type invalidUTF8Error struct{}

func (e *invalidUTF8Error) Error() string {
	return "file is not valid UTF-8"
}
