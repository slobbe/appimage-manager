package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var version = "dev"

type commandError struct {
	code int
}

func (err commandError) Error() string {
	return fmt.Sprintf("command exited with code %d", err.code)
}

func Run(ctx context.Context, buildVersion string, args []string, stdout io.Writer, stderr io.Writer) error {
	version = strings.TrimSpace(buildVersion)
	if version == "" {
		version = "dev"
	}

	root := newRootCommand(version)
	root.SetOut(stdout)
	root.SetErr(stderr)

	if handled, err := maybeRunExplicitHelp(ctx, root, args); handled {
		if err != nil {
			return commandError{code: renderCommandError(root, args, err)}
		}
		return nil
	}

	root.SetArgs(args)
	root.SetVersionTemplate("{{.Version}}\n")

	if err := root.ExecuteContext(ctx); err != nil {
		return commandError{code: renderCommandError(root, args, err)}
	}

	return nil
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(commandError); ok {
		return exitErr.code
	}
	return exitSoftware
}

func ShouldGenerateDocsFromEnv() bool {
	return strings.TrimSpace(os.Getenv("AIM_MAN_OUTPUT")) != "" || strings.TrimSpace(os.Getenv("AIM_COMPLETION_DIR")) != ""
}

func GenerateDocs(buildVersion string) error {
	version = strings.TrimSpace(buildVersion)
	if version == "" {
		version = "dev"
	}

	root := newRootCommand(version)

	outputPath := strings.TrimSpace(os.Getenv("AIM_MAN_OUTPUT"))
	if outputPath != "" {
		manPage, err := renderManPage(root, 1)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(outputPath, []byte(manPage), 0o644); err != nil {
			return err
		}
	} else {
		for _, cmd := range documentedCommands(root) {
			manPage, err := renderManPage(cmd, 1)
			if err != nil {
				return err
			}

			path := manPagePathForCommand(root, cmd)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(manPage), 0o644); err != nil {
				return err
			}
		}
	}

	completionDir := strings.TrimSpace(os.Getenv("AIM_COMPLETION_DIR"))
	if completionDir == "" {
		return nil
	}

	return writeCompletionFiles(root, completionDir)
}
