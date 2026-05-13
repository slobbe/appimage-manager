package filesystem

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"
)

func EnsureDir(path string) error {
	if path == "" {
		return fmt.Errorf("directory cannot be empty")
	}
	return os.MkdirAll(path, 0o755)
}

func RequireDir(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to access directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}
	return nil
}

func RequireRegularFile(path string, subject string) (os.FileInfo, error) {
	if subject == "" {
		subject = "path"
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to access %s: %w", subject, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory, not a file: %s", subject, path)
	}

	return info, nil
}

func ResolveRegularFile(path string, subject string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s %s: %w", subject, path, err)
	}
	if _, err := RequireRegularFile(resolved, subject); err != nil {
		return "", err
	}

	return resolved, nil
}

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

func ReadTextFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("failed to access file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("path is a directory, not a file: %s", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	if !utf8.Valid(content) {
		return "", fmt.Errorf("file is not valid UTF-8")
	}

	return string(content), nil
}

func ReplaceSymlink(src string, linkPath string) error {
	_ = os.Remove(linkPath)
	if err := EnsureDir(filepath.Dir(linkPath)); err != nil {
		return err
	}
	return os.Symlink(src, linkPath)
}

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
