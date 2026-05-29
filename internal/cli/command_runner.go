package cli

import (
	"context"

	"github.com/spf13/cobra"
)

type commandRunOptions struct {
	RequiresRuntimeDirs bool
	RequiresWriteLock   bool
}

func runCommand[T any](cmd *cobra.Command, opts commandRunOptions, execute func(context.Context) (*T, error), render func(*T) error) error {
	if opts.RequiresRuntimeDirs {
		if err := mustEnsureRuntimeDirs(); err != nil {
			return err
		}
	}

	ctx := context.Background()
	if cmd != nil && cmd.Context() != nil {
		ctx = cmd.Context()
	}

	var result *T
	run := func() error {
		var err error
		result, err = execute(ctx)
		return err
	}

	if opts.RequiresWriteLock {
		if err := withStateWriteLock(cmd, run); err != nil {
			return err
		}
	} else if err := run(); err != nil {
		return err
	}

	return render(result)
}
