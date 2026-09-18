package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/slobbe/appimage-manager/internal/app"

	"golang.org/x/sys/unix"
)

type MutationLocker struct {
	Path string
}

func NewMutationLocker(path string) MutationLocker {
	return MutationLocker{Path: path}
}

var _ app.MutationLocker = MutationLocker{}

func (l MutationLocker) Lock(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return nil, fmt.Errorf("create mutation lock directory: %w", err)
	}
	file, err := os.OpenFile(l.Path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open mutation lock %q: %w", l.Path, err)
	}
	for {
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() {
				_ = unix.Flock(int(file.Fd()), unix.LOCK_UN)
				_ = file.Close()
			}, nil
		}
		if err != unix.EWOULDBLOCK && err != unix.EAGAIN && err != unix.EINTR {
			_ = file.Close()
			return nil, fmt.Errorf("acquire mutation lock %q: %w", l.Path, err)
		}
		timer := time.NewTimer(lockRetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = file.Close()
			return nil, fmt.Errorf("acquire mutation lock %q: %w", l.Path, ctx.Err())
		case <-timer.C:
		}
	}
}
