package list

import (
	"context"
	"fmt"
	"io"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
	"github.com/slobbe/appimage-manager/internal/cli/output"

	"github.com/spf13/cobra"
)

type service interface {
	List(ctx context.Context, req app.ListRequest) (app.ListResult, error)
}

func NewCommand(rt *clienv.Runtime, service service) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all AppImages",
		Long:    "List all AppImages.",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := service.List(cmd.Context(), app.ListRequest{})
			if err != nil {
				return err
			}

			return output.Write(
				cmd.OutOrStdout(),
				rt.Config.JSON,
				result,
				func(w io.Writer) error {
					return writeTable(w, result.Items, rt)
				},
			)
		},
	}

	return cmd
}

func writeTable(w io.Writer, items []app.ListItem, rt *clienv.Runtime) error {
	idWidth := len("ID")
	nameWidth := len("Name")

	for _, item := range items {
		idWidth = max(idWidth, len(item.ID))
		nameWidth = max(nameWidth, len(item.Name))
	}

	const gap = 2
	format := fmt.Sprintf("%%-%ds%%-%ds%%s\n", idWidth+gap, nameWidth+gap)

	if _, err := fmt.Fprint(w, rt.Emphasis(w, fmt.Sprintf(format, "ID", "Name", "Version"))); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(w, format, item.ID, item.Name, item.Version); err != nil {
			return err
		}
	}

	return nil
}
