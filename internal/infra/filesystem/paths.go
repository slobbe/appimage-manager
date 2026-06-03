package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
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
