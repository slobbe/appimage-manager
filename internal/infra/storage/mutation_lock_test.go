package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestMutationLockerSerializesAndRespectsCancellation(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "operation.lock")
	unlock, err := NewMutationLocker(path).Lock(context.Background())
	if err != nil {
		t.Fatalf("first Lock() error = %v", err)
	}
	defer unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = NewMutationLocker(path).Lock(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Lock() error = %v, want context deadline", err)
	}
}
