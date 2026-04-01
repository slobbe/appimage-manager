//go:build !docgen

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := newRootCommand(version)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)

	if handled, err := maybeRunExplicitHelp(ctx, root, os.Args[1:]); handled {
		if err != nil {
			os.Exit(renderCommandError(root, os.Args[1:], err))
		}
		return
	}

	root.SetArgs(os.Args[1:])
	root.SetVersionTemplate("{{.Version}}\n")

	if err := root.ExecuteContext(ctx); err != nil {
		os.Exit(renderCommandError(root, os.Args[1:], err))
	}
}
