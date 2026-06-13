package remove

import (
	"fmt"
	"io"

	"aim/internal/app"
	"aim/internal/cli/activity"
	"aim/internal/cli/clienv"
	"aim/internal/cli/output"

	"github.com/spf13/cobra"
)

func NewCommand(rt *clienv.Runtime, service app.Remover) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <appimage>",
		Aliases: []string{"rm"},
		Short:   "Remove an AppImage.",
		Long:    "Remove an AppImage.",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reporter := activity.NewReporter(cmd.ErrOrStderr(), !rt.Config.JSON)

			req := app.RemoveRequest{
				Name:     args[0],
				Activity: reporter,
			}

			if err := service.Remove(cmd.Context(), req); err != nil {
				reporter.Wait()
				return err
			}
			reporter.Wait()

			return output.Write(
				cmd.OutOrStdout(),
				rt.Config.JSON,
				struct {
					Status string `json:"status"`
					Action string `json:"action"`
					Name   string `json:"name"`
				}{
					Status: "ok",
					Action: "remove",
					Name:   req.Name,
				},
				func(w io.Writer) error {
					fmt.Fprintf(w, "\033[32mSuccessfully removed %s!\033[0m\n", req.Name)
					return nil
				},
			)
		},
	}

	return cmd
}
