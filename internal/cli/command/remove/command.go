package remove

import (
	"context"
	"fmt"
	"io"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/activity"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
	"github.com/slobbe/appimage-manager/internal/cli/output"

	"github.com/spf13/cobra"
)

type service interface {
	Remove(ctx context.Context, req app.RemoveRequest) error
}

func NewCommand(rt *clienv.Runtime, service service) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <appimage>",
		Aliases: []string{"rm"},
		Short:   "Remove an AppImage",
		Long:    "Remove an AppImage.",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			reporter := activity.NewReporter(cmd.ErrOrStderr(), rt.ActivityEnabled(cmd.ErrOrStderr()))

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
					_, err := fmt.Fprintln(w, rt.Success(w, fmt.Sprintf("Successfully removed %s!", req.Name)))
					return err
				},
			)
		},
	}

	return cmd
}
