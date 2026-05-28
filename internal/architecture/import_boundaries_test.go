package architecture_test

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
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

func TestCLIInfraImportsStayInRuntimeComposition(t *testing.T) {
	root := repoRoot(t)
	cliDir := filepath.Join(root, "internal", "cli")
	entries, err := os.ReadDir(cliDir)
	if err != nil {
		t.Fatalf("read cli directory: %v", err)
	}

	allowedFiles := map[string]bool{
		"runtime.go":        true,
		"runtime_wiring.go": true,
	}

	var violations []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || allowedFiles[name] {
			continue
		}

		path := filepath.Join(cliDir, name)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s imports: %v", name, err)
		}
		for _, imported := range file.Imports {
			importPath := strings.Trim(imported.Path.Value, "\"")
			if strings.Contains(importPath, "/internal/infra") {
				violations = append(violations, filepath.ToSlash(filepath.Join("internal", "cli", name))+" imports "+importPath)
			}
		}
	}

	if len(violations) > 0 {
		t.Fatalf("cli infra imports outside runtime composition files:\n%s", strings.Join(violations, "\n"))
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
