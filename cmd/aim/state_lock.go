package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/slobbe/appimage-manager/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

func withStateWriteLock(cmd *cobra.Command, fn func() error) error {
	if err := os.MkdirAll(config.ConfigDir, 0o755); err != nil {
		return wrapWriteError(err)
	}

	lockPath := filepath.Join(config.ConfigDir, "state.lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return wrapWriteError(err)
	}
	defer file.Close()

	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return noPermError(fmt.Errorf("another aim process is already modifying this AIM state root; wait for it to finish and try again"))
	}
	defer func() {
		_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
	}()

	return fn()
}
