//go:build !docgen

package main

import (
	"context"
	"os"
)

func main() {
	root := newRootCommand(version)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)

	if handled, err := maybeRunExplicitHelp(context.Background(), root, os.Args[1:]); handled {
		if err != nil {
			os.Exit(renderCommandError(root, os.Args[1:], err))
		}
		return
	}

	root.SetArgs(os.Args[1:])
	root.SetVersionTemplate("{{.Version}}\n")

	if err := root.ExecuteContext(context.Background()); err != nil {
		os.Exit(renderCommandError(root, os.Args[1:], err))
	}
}
