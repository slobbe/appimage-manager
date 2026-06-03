package selfupdate

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

type Client struct {
	HTTPClient     *http.Client
	ShellCommand   string
	ExecutablePath func() (string, error)
	EvalSymlinks   func(string) (string, error)
	VersionCommand func(context.Context, string) (string, error)
}

type latestReleaseResponse struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func (c Client) FetchLatestReleaseTag(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := c.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("latest release request failed with status %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	tag, err := latestReleaseTagFromJSON(body)
	if err != nil {
		return "", err
	}
	return tag, nil
}

func latestReleaseTagFromJSON(body []byte) (string, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "", fmt.Errorf("latest release response was empty")
	}

	if trimmed[0] == '[' {
		var payload []latestReleaseResponse
		if err := json.Unmarshal(trimmed, &payload); err != nil {
			return "", err
		}
		for _, release := range payload {
			if release.Draft {
				continue
			}
			if tag := strings.TrimSpace(release.TagName); tag != "" {
				return tag, nil
			}
		}
		return "", fmt.Errorf("release list response did not include a selectable tag_name")
	}

	var payload latestReleaseResponse
	if err := json.Unmarshal(trimmed, &payload); err != nil {
		return "", err
	}
	tag := strings.TrimSpace(payload.TagName)
	if tag == "" {
		return "", fmt.Errorf("latest release response did not include a tag_name")
	}
	return tag, nil
}

func (c Client) RunInstallerScript(ctx context.Context, scriptURL string, tempDir func() (string, error), env map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scriptURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.client().Do(req)
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

	scriptPath := filepath.Join(dir, "aim-self-update-installer.sh")
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

	return c.runShellScript(ctx, scriptPath, env)
}

func (c Client) ResolveInstalledPath() (string, error) {
	execPath, err := c.executablePath()()
	if err != nil {
		return "", err
	}

	if resolvedPath, err := c.evalSymlinks()(execPath); err == nil && strings.TrimSpace(resolvedPath) != "" {
		execPath = resolvedPath
	}

	return execPath, nil
}

func (c Client) ReadInstalledVersion(ctx context.Context, binaryPath string) (string, error) {
	output, err := c.versionCommand()(ctx, binaryPath)
	if err != nil {
		return "", err
	}

	version := strings.TrimSpace(output)
	if version == "" {
		return "", fmt.Errorf("installed version command returned empty output")
	}

	return version, nil
}

func (c Client) runShellScript(ctx context.Context, scriptPath string, env map[string]string) error {
	cmd := exec.CommandContext(ctx, c.shellCommand(), scriptPath)
	if len(env) > 0 {
		cmd.Env = os.Environ()
		for key, value := range env {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
		}
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(output.String())
		if message == "" {
			return fmt.Errorf("self-update via installer failed: %w", err)
		}
		return fmt.Errorf("self-update via installer failed: %w: %s", err, message)
	}

	return nil
}

func (c Client) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c Client) shellCommand() string {
	if strings.TrimSpace(c.ShellCommand) != "" {
		return c.ShellCommand
	}
	return "/bin/sh"
}

func (c Client) executablePath() func() (string, error) {
	if c.ExecutablePath != nil {
		return c.ExecutablePath
	}
	return os.Executable
}

func (c Client) evalSymlinks() func(string) (string, error) {
	if c.EvalSymlinks != nil {
		return c.EvalSymlinks
	}
	return filepath.EvalSymlinks
}

func (c Client) versionCommand() func(context.Context, string) (string, error) {
	if c.VersionCommand != nil {
		return c.VersionCommand
	}
	return RunVersionCommand
}

func RunVersionCommand(ctx context.Context, binaryPath string) (string, error) {
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
