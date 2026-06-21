package selfupdate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"

	"github.com/slobbe/appimage-manager/internal/app"
)

const installScriptURLFormat = "https://raw.githubusercontent.com/slobbe/appimage-manager/%s/scripts/install.sh"

// Installer runs the hosted install script to replace the current aim binary.
type Installer struct{}

var _ app.SelfUpdater = Installer{}

func (i Installer) Install(ctx context.Context, version string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return errors.New("self-update version is required")
	}

	script, err := i.fetchInstallScript(ctx, version)
	if err != nil {
		return err
	}
	defer script.Close()

	output, err := runShellCommand(ctx, "sh", script, []string{"AIM_VERSION=" + strings.TrimPrefix(version, "v")})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("run self-update installer: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return nil
}

func runShellCommand(ctx context.Context, name string, stdin io.Reader, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name)
	cmd.Stdin = stdin
	cmd.Env = append(cmd.Environ(), env...)
	return cmd.CombinedOutput()
}

func (i Installer) fetchInstallScript(ctx context.Context, version string) (io.ReadCloser, error) {
	url := installScriptURLForVersion(version)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create install script request: %w", err)
	}
	req.Header.Set("User-Agent", "aim")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("download install script %q: %w", url, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		resp.Body.Close()
		return nil, fmt.Errorf("download install script %q: server returned %s", url, resp.Status)
	}

	return resp.Body, nil
}

func installScriptURLForVersion(version string) string {
	tag := strings.TrimSpace(version)
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	return fmt.Sprintf(installScriptURLFormat, tag)
}
