package fileutil

import (
	"context"
	"fmt"
	"io"
	"os"
)

func CopyFile(ctx context.Context, sourcePath string, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	info, err := source.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("source path %q is a directory", sourcePath)
	}

	temporaryDestination := destination + ".tmp"
	dest, err := os.OpenFile(temporaryDestination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(dest, source)
	closeErr := dest.Close()
	if copyErr != nil {
		_ = os.Remove(temporaryDestination)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(temporaryDestination)
		return closeErr
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(temporaryDestination)
		return err
	}

	return os.Rename(temporaryDestination, destination)
}
