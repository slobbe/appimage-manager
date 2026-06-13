package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"

	"github.com/slobbe/appimage-manager/internal/cli/command/add"
	"github.com/slobbe/appimage-manager/internal/cli/command/gen"
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
		fmt.Fprintln(errOut, err)
		return 1
	}

	return 0
}

func NewRootCommand(rt *clienv.Runtime, service app.Service, version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "aim",
		Short:         "An AppImage Manager.",
		Long:          "aim is a CLI tool for managing AppImage files. Integrate, update, and manage your AppImages.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.SetVersionTemplate("{{.Version}}\n")

	cmd.PersistentFlags().BoolVar(
		&rt.Config.JSON,
		"json",
		false,
		"output command results as JSON",
	)

	cmd.AddCommand(add.NewCommand(rt, service))
	cmd.AddCommand(remove.NewCommand(rt, service))
	cmd.AddCommand(update.NewCommand(rt, service))
	cmd.AddCommand(list.NewCommand(rt, service))
	cmd.AddCommand(info.NewCommand(rt, service))
	cmd.AddCommand(selfupdate.NewCommand(rt, service))
	cmd.AddCommand(paths.NewCommand(rt, service))
	cmd.AddCommand(gen.NewCommand(cmd))

	return cmd
}
