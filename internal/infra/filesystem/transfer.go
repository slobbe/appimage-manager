package filesystem

import (
	"io"
	"os"
	"path/filepath"
)

func Move(src string, dst string) (string, error) {
	if err := EnsureDir(filepath.Dir(dst)); err != nil {
		return src, err
	}
	if err := os.Rename(src, dst); err != nil {
		return src, err
	}

	return dst, nil
}

func Copy(src string, dst string) (string, error) {
	if err := EnsureDir(filepath.Dir(dst)); err != nil {
		return src, err
	}

	in, err := os.Open(src)
	if err != nil {
		return src, err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return src, err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return dst, err
}

func MakeExecutable(path string) error {
	return os.Chmod(path, 0o755)
}
