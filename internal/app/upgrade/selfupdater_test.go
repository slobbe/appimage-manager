package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	upgradeHTTPClient        = &http.Client{}
	upgradeShellCommand      = "/bin/sh"
	upgradeExecutablePath    = os.Executable
	upgradeEvalSymlinks      = filepath.EvalSymlinks
	upgradeRunVersionCommand = runVersionCommandForTest
)

type testSelfUpdater struct{}

func (testSelfUpdater) FetchLatestReleaseTag(ctx context.Context, releaseURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := upgradeHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("latest release request failed with status %s", resp.Status)
	}
	var payload testLatestReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	tag := strings.TrimSpace(payload.TagName)
	if tag == "" {
		return "", fmt.Errorf("latest release response did not include a tag_name")
	}
	return tag, nil
}

func (testSelfUpdater) RunInstallerScript(ctx context.Context, scriptURL string, tempDir func() (string, error)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scriptURL, nil)
	if err != nil {
		return err
	}
	resp, err := upgradeHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("installer script download failed with status %s", resp.Status)
	}
	dir, err := tempDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	scriptPath := filepath.Join(dir, "aim-upgrade-installer.sh")
	scriptFile, err := os.OpenFile(scriptPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		return err
	}
	if _, err := io.Copy(scriptFile, resp.Body); err != nil {
		_ = scriptFile.Close()
		return err
	}
	if err := scriptFile.Close(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, upgradeShellCommand, scriptPath)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(output.String())
		if message == "" {
			return fmt.Errorf("upgrade via installer failed: %w", err)
		}
		return fmt.Errorf("upgrade via installer failed: %w: %s", err, message)
	}
	return nil
}

func (testSelfUpdater) ResolveInstalledPath() (string, error) {
	execPath, err := upgradeExecutablePath()
	if err != nil {
		return "", err
	}
	if resolvedPath, err := upgradeEvalSymlinks(execPath); err == nil && strings.TrimSpace(resolvedPath) != "" {
		execPath = resolvedPath
	}
	return execPath, nil
}

func (testSelfUpdater) ReadInstalledVersion(ctx context.Context, binaryPath string) (string, error) {
	output, err := upgradeRunVersionCommand(ctx, binaryPath)
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(output)
	if version == "" {
		return "", fmt.Errorf("installed version command returned empty output")
	}
	return version, nil
}

func runVersionCommandForTest(ctx context.Context, binaryPath string) (string, error) {
	cmd := exec.CommandContext(ctx, binaryPath, "--version")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return stdout.String(), nil
}
