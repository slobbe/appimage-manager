package architecture_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type goPackage struct {
	ImportPath string
	Imports    []string
}

func TestLayerImportBoundaries(t *testing.T) {
	root := repoRoot(t)
	cmd := exec.Command("go", "list", "-json", "./internal/...")
	cmd.Dir = root

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list failed: %v\n%s", err, exitErr.Stderr)
		}
		t.Fatalf("go list failed: %v", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output))
	var violations []string
	for {
		var pkg goPackage
		if err := decoder.Decode(&pkg); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode go list output: %v", err)
		}
		violations = append(violations, packageBoundaryViolations(pkg)...)
	}

	if len(violations) > 0 {
		t.Fatalf("layer import boundary violations:\n%s", strings.Join(violations, "\n"))
	}
}

func packageBoundaryViolations(pkg goPackage) []string {
	var forbidden []string
	switch {
	case strings.Contains(pkg.ImportPath, "/internal/domain"):
		forbidden = []string{"/internal/app", "/internal/infra", "/internal/cli"}
	case strings.Contains(pkg.ImportPath, "/internal/app"):
		forbidden = []string{"/internal/infra", "/internal/cli"}
	case strings.Contains(pkg.ImportPath, "/internal/infra"):
		forbidden = []string{"/internal/cli"}
	default:
		return nil
	}

	var violations []string
	for _, imported := range pkg.Imports {
		for _, boundary := range forbidden {
			if strings.Contains(imported, boundary) {
				violations = append(violations, pkg.ImportPath+" imports "+imported)
			}
		}
	}
	return violations
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
