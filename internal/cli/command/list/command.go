package list

import (
	"fmt"
	"io"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
	"github.com/slobbe/appimage-manager/internal/cli/output"

	"github.com/spf13/cobra"
)

const (
	bold  = "\033[1m"
	reset = "\033[0m"
)

func NewCommand(rt *clienv.Runtime, service app.Service) *cobra.Command {
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
				struct {
					Items []listItemJSON `json:"items"`
				}{
					Items: toJSON(result.Items),
				},
				func(w io.Writer) error {
					return writeTable(w, result.Items)
				},
			)
		},
	}

	return cmd
}

type listItemJSON struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

func toJSON(items []app.ListItem) []listItemJSON {
	result := make([]listItemJSON, 0, len(items))
	for _, item := range items {
		result = append(result, listItemJSON{
			ID:      item.ID,
			Name:    item.Name,
			Version: item.Version,
		})
	}

	return result
}

func writeTable(w io.Writer, items []app.ListItem) error {
	idWidth := len("ID")
	nameWidth := len("Name")

	for _, item := range items {
		idWidth = max(idWidth, len(item.ID))
		nameWidth = max(nameWidth, len(item.Name))
	}

	const gap = 2
	format := fmt.Sprintf("%%-%ds%%-%ds%%s\n", idWidth+gap, nameWidth+gap)

	fmt.Fprintf(w, bold+format+reset, "ID", "Name", "Version")
	for _, item := range items {
		fmt.Fprintf(w, format, item.ID, item.Name, item.Version)
	}

	return nil
}
