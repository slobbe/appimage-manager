package core

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	upgradeRepoSlug   = "slobbe/appimage-manager"
	upgradeHTTPClient = sharedHTTPClient

	upgradeInstallScriptURL = func(repoSlug string) string {
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/main/scripts/install.sh", repoSlug)
	}
	upgradeShellCommand       = "/bin/sh"
	upgradeRunInstallerScript = runInstallerScript
	upgradeExecutablePath     = os.Executable
	upgradeEvalSymlinks       = filepath.EvalSymlinks
	upgradeRunVersionCommand  = runInstalledVersionCommand
)

type InstallerUpgradeResult struct {
	PreviousVersion  string
	InstalledVersion string
}

func UpgradeViaInstaller(ctx context.Context, currentVersion string) (*InstallerUpgradeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	result := &InstallerUpgradeResult{
		PreviousVersion: strings.TrimSpace(currentVersion),
	}

	if err := upgradeRunInstallerScript(ctx, upgradeInstallScriptURL(upgradeRepoSlug)); err != nil {
		return nil, err
	}

	binaryPath, err := resolveInstalledAimPath()
	if err != nil {
		return result, nil
	}

	installedVersion, err := readInstalledAimVersion(ctx, binaryPath)
	if err != nil {
		return result, nil
	}
	result.InstalledVersion = installedVersion

	return result, nil
}

func runInstallerScript(ctx context.Context, scriptURL string) error {
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

	tempDir, err := os.MkdirTemp("", "aim-upgrade-installer-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	scriptPath := filepath.Join(tempDir, "install.sh")
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

func resolveInstalledAimPath() (string, error) {
	execPath, err := upgradeExecutablePath()
	if err != nil {
		return "", err
	}

	if resolvedPath, err := upgradeEvalSymlinks(execPath); err == nil && strings.TrimSpace(resolvedPath) != "" {
		execPath = resolvedPath
	}

	return execPath, nil
}

func readInstalledAimVersion(ctx context.Context, binaryPath string) (string, error) {
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

func runInstalledVersionCommand(ctx context.Context, binaryPath string) (string, error) {
	cmd := exec.CommandContext(ctx, binaryPath, "--version")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, message)
	}

	return stdout.String(), nil
}
