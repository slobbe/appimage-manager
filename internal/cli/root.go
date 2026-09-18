package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
	"github.com/slobbe/appimage-manager/internal/cli/presentation"

	"github.com/slobbe/appimage-manager/internal/cli/command/add"
	"github.com/slobbe/appimage-manager/internal/cli/command/gen"
	"github.com/slobbe/appimage-manager/internal/cli/command/id"
	"github.com/slobbe/appimage-manager/internal/cli/command/info"
	"github.com/slobbe/appimage-manager/internal/cli/command/list"
	"github.com/slobbe/appimage-manager/internal/cli/command/paths"
	"github.com/slobbe/appimage-manager/internal/cli/command/remove"
	"github.com/slobbe/appimage-manager/internal/cli/command/selfupdate"
	"github.com/slobbe/appimage-manager/internal/cli/command/update"

	"github.com/spf13/cobra"
)

func Execute(
	ctx context.Context,
	args []string,
	out io.Writer,
	errOut io.Writer,
	service app.Service,
	version string,
) int {
	rt := clienv.New(out, errOut)

	cmd := NewRootCommand(rt, service, version)
	cmd.SetContext(ctx)
	cmd.SetArgs(args)
	cmd.SetOut(out)
	cmd.SetErr(errOut)

	if err := cmd.ExecuteContext(ctx); err != nil {
		code, message := clienv.ResolveExit(err)
		if code == clienv.ExitFailure && strings.HasPrefix(err.Error(), "unknown command ") {
			code = clienv.ExitUsage
		}
		if message != nil {
			fmt.Fprintln(errOut, message)
		}
		return code
	}

	return clienv.ExitSuccess
}

func NewRootCommand(rt *clienv.Runtime, service app.Service, version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "aim",
		Short:         "AppImage Manager.",
		Long:          "aim is a CLI tool for managing AppImages. Integrate, update, and manage AppImages.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := presentation.ValidateColorMode(rt.Config.Color); err != nil {
				return clienv.UsageError(err)
			}
			return nil
		},
	}

	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return clienv.UsageError(err)
	})

	cmd.PersistentFlags().BoolVar(
		&rt.Config.JSON,
		"json",
		false,
		"output command results as JSON",
	)
	cmd.PersistentFlags().StringVar(
		&rt.Config.Color,
		"color",
		presentation.ColorAuto,
		"color output: auto, always, or never",
	)
	cmd.PersistentFlags().BoolVar(
		&rt.Config.Yes,
		"yes",
		false,
		"automatically confirm mutation prompts",
	)
	cmd.PersistentFlags().BoolVar(
		&rt.Config.NonInteractive,
		"non-interactive",
		false,
		"fail instead of prompting for input",
	)

	cmd.CompletionOptions.HiddenDefaultCmd = true

	cmd.AddCommand(add.NewCommand(rt, service))
	cmd.AddCommand(remove.NewCommand(rt, service))
	cmd.AddCommand(update.NewCommand(rt, service))
	cmd.AddCommand(id.NewCommand(rt, service))
	cmd.AddCommand(list.NewCommand(rt, service))
	cmd.AddCommand(info.NewCommand(rt, service))
	cmd.AddCommand(selfupdate.NewCommand(rt, service))
	cmd.AddCommand(paths.NewCommand(rt, service))
	cmd.AddCommand(gen.NewCommand(cmd))
	cmd.InitDefaultHelpCmd()
	configureHelpArguments(cmd)
	markArgumentErrorsAsUsage(cmd)

	return cmd
}

func configureHelpArguments(root *cobra.Command) {
	help, _, err := root.Find([]string{"help"})
	if err != nil || help == root {
		return
	}
	help.Args = func(_ *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		target, remaining, err := root.Find(args)
		if err != nil || target == root || target == help || len(remaining) > 0 {
			return clienv.UsageError(fmt.Errorf("unknown help topic %q", strings.Join(args, " ")))
		}
		return nil
	}
}

func markArgumentErrorsAsUsage(cmd *cobra.Command) {
	if cmd.Args != nil {
		validate := cmd.Args
		cmd.Args = func(cmd *cobra.Command, args []string) error {
			if err := validate(cmd, args); err != nil {
				return clienv.UsageError(err)
			}
			return nil
		}
	}
	for _, child := range cmd.Commands() {
		markArgumentErrorsAsUsage(child)
	}
}
