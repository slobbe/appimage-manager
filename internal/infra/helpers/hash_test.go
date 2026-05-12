package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSha256AndSha1MatchesIndividualFunctions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.bin")
	if err := os.WriteFile(path, []byte("hello world\n"), 0o644); err != nil {
		t.Fatalf("failed to write sample file: %v", err)
	}

	combinedSHA256, combinedSHA1, err := Sha256AndSha1(path)
	if err != nil {
		t.Fatalf("Sha256AndSha1 returned error: %v", err)
	}

	separateSHA256, err := Sha256File(path)
	if err != nil {
		t.Fatalf("Sha256File returned error: %v", err)
	}

	separateSHA1, err := Sha1(path)
	if err != nil {
		t.Fatalf("Sha1 returned error: %v", err)
	}

	if combinedSHA256 != separateSHA256 {
		t.Fatalf("combined sha256 = %q, want %q", combinedSHA256, separateSHA256)
	}

	if combinedSHA1 != separateSHA1 {
		t.Fatalf("combined sha1 = %q, want %q", combinedSHA1, separateSHA1)
	}
}

func TestSha256AndSha1MissingFile(t *testing.T) {
	if _, _, err := Sha256AndSha1(filepath.Join(t.TempDir(), "missing.bin")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
