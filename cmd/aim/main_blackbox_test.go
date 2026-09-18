package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCompiledCLIPhaseOneSmoke(t *testing.T) {
	binary := buildCLI(t)
	home := t.TempDir()
	env := isolatedEnvironment(home)

	t.Run("help and version use stdout", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			args []string
			want string
		}{
			{name: "help", args: []string{"--help"}, want: "Usage:"},
			{name: "help command", args: []string{"help", "list"}, want: "List all AppImages."},
			{name: "version", args: []string{"--version"}, want: "dev"},
			{name: "command help", args: []string{"list", "--help"}, want: "List all AppImages."},
		} {
			t.Run(tc.name, func(t *testing.T) {
				result := runCLI(t, binary, env, tc.args...)
				if result.exitCode != 0 {
					t.Fatalf("exit code = %d, want 0; stderr = %q", result.exitCode, result.stderr)
				}
				if !strings.Contains(result.stdout, tc.want) {
					t.Fatalf("stdout = %q, want substring %q", result.stdout, tc.want)
				}
				if result.stderr != "" {
					t.Fatalf("stderr = %q, want empty", result.stderr)
				}
			})
		}
	})

	t.Run("ordinary error has nonzero exit and stderr", func(t *testing.T) {
		result := runCLI(t, binary, env, "list", "unexpected")
		if result.exitCode != 2 {
			t.Fatalf("exit code = %d, want 2", result.exitCode)
		}
		if result.stdout != "" {
			t.Fatalf("stdout = %q, want empty", result.stdout)
		}
		if !strings.Contains(result.stderr, `unknown command "unexpected"`) {
			t.Fatalf("stderr = %q, want ordinary command error", result.stderr)
		}
	})

	t.Run("invalid usage exits two", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			args []string
		}{
			{name: "unknown command", args: []string{"not-a-command"}},
			{name: "unknown help topic", args: []string{"help", "nope"}},
			{name: "excess help arguments", args: []string{"help", "list", "extra"}},
			{name: "invalid color", args: []string{"--color=sometimes", "list"}},
			{name: "missing generator directory", args: []string{"gen", "man"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				result := runCLI(t, binary, env, tc.args...)
				if result.exitCode != 2 {
					t.Fatalf("exit code = %d, want 2; stderr = %q", result.exitCode, result.stderr)
				}
				if result.stdout != "" {
					t.Fatalf("stdout = %q, want empty", result.stdout)
				}
				if result.stderr == "" {
					t.Fatal("stderr is empty, want usage diagnostic")
				}
			})
		}
	})

	t.Run("JSON list is isolated on stdout", func(t *testing.T) {
		result := runCLI(t, binary, env, "--json", "list")
		if result.exitCode != 0 {
			t.Fatalf("exit code = %d, want 0; stderr = %q", result.exitCode, result.stderr)
		}
		if result.stderr != "" {
			t.Fatalf("stderr = %q, want empty", result.stderr)
		}
		var payload struct {
			Items []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
			t.Fatalf("stdout is not JSON: %v; stdout = %q", err, result.stdout)
		}
		if len(payload.Items) != 0 {
			t.Fatalf("items = %s, want empty fresh state", payload.Items)
		}
	})

	t.Run("generation commands create usable artifacts", func(t *testing.T) {
		manDir := filepath.Join(t.TempDir(), "man")
		result := runCLI(t, binary, env, "gen", "man", "--dir", manDir)
		if result.exitCode != 0 || result.stderr != "" {
			t.Fatalf("gen man = (%d, %q, %q), want success", result.exitCode, result.stdout, result.stderr)
		}
		assertNonemptyFile(t, filepath.Join(manDir, "aim.1"))

		for _, shell := range []struct {
			name string
			file string
		}{
			{name: "bash", file: "aim.bash"},
			{name: "zsh", file: "_aim"},
			{name: "fish", file: "aim.fish"},
			{name: "powershell", file: "aim.ps1"},
		} {
			t.Run(shell.name, func(t *testing.T) {
				dir := filepath.Join(t.TempDir(), shell.name)
				result := runCLI(t, binary, env, "gen", "completion", shell.name, "--dir", dir)
				if result.exitCode != 0 || result.stderr != "" {
					t.Fatalf("completion = (%d, %q, %q), want success", result.exitCode, result.stdout, result.stderr)
				}
				assertNonemptyFile(t, filepath.Join(dir, shell.file))
			})
		}
	})

	t.Run("read-only commands do not persist state", func(t *testing.T) {
		for _, path := range []string{
			filepath.Join(home, "config"),
			filepath.Join(home, "data"),
			filepath.Join(home, "cache"),
		} {
			entries, err := os.ReadDir(path)
			if err != nil {
				t.Fatalf("ReadDir(%q): %v", path, err)
			}
			if len(entries) != 0 {
				t.Fatalf("%s contains %v, want no persisted CLI state", path, entries)
			}
		}
	})
}

func buildCLI(t *testing.T) string {
	t.Helper()
	name := "aim"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", path, ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}
	return path
}

func isolatedEnvironment(root string) []string {
	config := filepath.Join(root, "config")
	data := filepath.Join(root, "data")
	cache := filepath.Join(root, "cache")
	for _, path := range []string{config, data, cache} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			panic(err)
		}
	}
	return []string{
		"HOME=" + root,
		"XDG_CONFIG_HOME=" + config,
		"XDG_DATA_HOME=" + data,
		"XDG_CACHE_HOME=" + cache,
		"PATH=" + os.Getenv("PATH"),
	}
}

type cliResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func runCLI(t *testing.T, binary string, env []string, args ...string) cliResult {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = env
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := cliResult{stdout: stdout.String(), stderr: stderr.String()}
	if err == nil {
		return result
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("run CLI: %v", err)
	}
	result.exitCode = exitErr.ExitCode()
	return result
}

func assertNonemptyFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q): %v", path, err)
	}
	if info.Size() == 0 {
		t.Fatalf("%s is empty", path)
	}
}
