package zsync

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Runner struct {
	LookPath       func(string) (string, error)
	CommandContext func(context.Context, string, ...string) *exec.Cmd
}

func (r Runner) Apply(ctx context.Context, currentPath, zsyncURL, destination string) error {
	currentPath = strings.TrimSpace(currentPath)
	zsyncURL = strings.TrimSpace(zsyncURL)
	destination = strings.TrimSpace(destination)
	if currentPath == "" {
		return fmt.Errorf("missing app exec path")
	}
	if zsyncURL == "" {
		return fmt.Errorf("missing zsync url")
	}
	if destination == "" {
		return fmt.Errorf("missing zsync destination")
	}

	lookPath := r.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	commandContext := r.CommandContext
	if commandContext == nil {
		commandContext = exec.CommandContext
	}

	binary, err := lookPath("zsync")
	if err != nil {
		return err
	}

	cmd := commandContext(ctx, binary, "-q", "-i", currentPath, "-o", destination, zsyncURL)
	cmd.Dir = filepath.Dir(destination)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, msg)
	}

	if _, err := os.Stat(destination); err != nil {
		return err
	}

	return nil
}
