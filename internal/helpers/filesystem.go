package util

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"
)

func Move(file string, dest string) (string, error) {
	if err := os.Rename(file, dest); err != nil {
		return file, err
	}

	return dest, nil
}

func Copy(src, dst string) (string, error) {
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

func MakeExecutable(src string) error {
	const execPerm = 0755
	if err := os.Chmod(src, execPerm); err != nil {
		return err
	}

	return nil
}

func MakeAbsolute(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return path, err
	}

	return filepath.Join(dir, path), nil
}

func ReadFileContents(src string) (string, error) {
	info, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("failed to access file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file: %s", src)
	}

	content, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	if !utf8.Valid(content) {
		return "", fmt.Errorf("file is not valid UTF-8")
	}
	
	return string(content), nil
}
