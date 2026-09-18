package paths

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
	Paths(ctx context.Context, req app.PathsRequest) (app.PathsResult, error)
}

func NewCommand(rt *clienv.Runtime, service service) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "paths",
		Short: "Show aim paths",
		Long:  "Show the config, AppImage, desktop entry, and icon paths used by aim.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := service.Paths(cmd.Context(), app.PathsRequest{})
			if err != nil {
				return err
			}

			return output.Write(
				cmd.OutOrStdout(),
				rt.Config.JSON,
				result,
				func(w io.Writer) error {
					_, err := fmt.Fprintf(
						w,
						"Config file:  %s\nAppImage dir: %s\nDesktop dir:  %s\nIcon dir:     %s\n",
						result.ConfigFile,
						result.AppImageDir,
						result.DesktopDir,
						result.IconDir,
					)
					return err
				},
			)
		},
	}

	return cmd
}
