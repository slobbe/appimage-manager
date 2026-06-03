package appimage

import (
	"bytes"
	"context"
	"debug/elf"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	fsys "github.com/slobbe/appimage-manager/internal/infra/filesystem"
)

type Extraction struct {
	Dir     string
	RootDir string
}

type CleanupFunc func()

func Inspect(ctx context.Context, src string) (*Extraction, CleanupFunc, error) {
	tempDir, err := os.MkdirTemp("", "aim-inspect-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = fsys.RemoveAll(tempDir)
	}

	copyPath := filepath.Join(tempDir, filepath.Base(src))
	if _, err := fsys.Copy(src, copyPath); err != nil {
		cleanup()
		return nil, nil, err
	}
	if err := fsys.MakeExecutable(copyPath); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to make executable: %w", err)
	}

	extraction, err := extractCombined(ctx, copyPath, tempDir)
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	return extraction, cleanup, nil
}

func Extract(ctx context.Context, src string, tempBaseDir string) (*Extraction, CleanupFunc, error) {
	srcFileName := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	tempDir := tempBaseDir + "-" + srcFileName
	if err := fsys.EnsureDir(tempDir); err != nil {
		return nil, nil, fmt.Errorf("failed to create temporary directory %s: %w", tempDir, err)
	}
	cleanup := func() {
		_ = fsys.RemoveAll(tempDir)
	}

	if err := fsys.MakeExecutable(src); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("failed to make executable: %w", err)
	}

	extraction, err := extractStreaming(ctx, src, tempDir)
	if err != nil {
		cleanup()
		return nil, nil, err
	}

	return extraction, cleanup, nil
}

func extractCombined(ctx context.Context, src string, dir string) (*Extraction, error) {
	extractCmd := exec.CommandContext(ctx, src, "--appimage-extract")
	extractCmd.Dir = dir

	out, err := extractCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w\nOutput: %s", err, string(out))
	}

	return verifiedExtraction(dir)
}

func extractStreaming(ctx context.Context, src string, dir string) (*Extraction, error) {
	extractCmd := exec.CommandContext(ctx, src, "--appimage-extract")
	extractCmd.Dir = dir
	extractCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var out bytes.Buffer
	extractCmd.Stdout = &out
	extractCmd.Stderr = &out

	if err := extractCmd.Start(); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, context.Canceled
		}
		return nil, fmt.Errorf("extraction failed: %w\nOutput: %s", err, out.String())
	}

	pid := extractCmd.Process.Pid
	done := make(chan struct{})
	if ctx != nil {
		go func() {
			select {
			case <-ctx.Done():
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			case <-done:
			}
		}()
	}

	err := extractCmd.Wait()
	close(done)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, context.Canceled
		}
		return nil, fmt.Errorf("extraction failed: %w\nOutput: %s", err, out.String())
	}

	return verifiedExtraction(dir)
}

func ExtractUpdateInfo(src string) (string, error) {
	if !strings.EqualFold(filepath.Ext(strings.TrimSpace(src)), ".AppImage") {
		return "", fmt.Errorf("source must be .AppImage file")
	}

	src, err := fsys.MakeAbsolute(src)
	if err != nil {
		return "", err
	}

	f, err := elf.Open(src)
	if err != nil {
		return "", err
	}
	defer f.Close()

	section := f.Section(".upd_info")
	if section == nil {
		return "", fmt.Errorf("no update information found in ELF headers")
	}

	data, err := section.Data()
	if err != nil {
		return "", err
	}

	strData := string(data)
	if i := strings.Index(strData, "\x00"); i != -1 {
		strData = strData[:i]
	}

	return strings.TrimSpace(strData), nil
}

func verifiedExtraction(dir string) (*Extraction, error) {
	root := filepath.Join(dir, "squashfs-root")
	if err := fsys.RequireDir(root); err != nil {
		return nil, fmt.Errorf("extraction verification failed: squashfs-root not found")
	}

	return &Extraction{Dir: dir, RootDir: root}, nil
}
