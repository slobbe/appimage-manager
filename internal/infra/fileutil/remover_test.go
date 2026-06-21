package fileutil

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoverRemovesArtifact(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "artifact")
	if err := os.WriteFile(path, []byte("artifact"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NewRemover().Remove(context.Background(), path); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat error = %v, want not exist", err)
	}
}

func TestRemoverIgnoresMissingArtifact(t *testing.T) {
	t.Parallel()
	if err := NewRemover().Remove(context.Background(), filepath.Join(t.TempDir(), "missing")); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
}

func TestRemoverValidatesArtifactPath(t *testing.T) {
	t.Parallel()
	if err := NewRemover().Remove(context.Background(), ""); err == nil {
		t.Fatal("Remove() error = nil, want error")
	}
}

func TestRemoverRespectsCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewRemover().Remove(ctx, "artifact"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Remove() error = %v, want context.Canceled", err)
	}
}
