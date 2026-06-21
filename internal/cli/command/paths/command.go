package paths

import (
	"fmt"
	"io"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
	"github.com/slobbe/appimage-manager/internal/cli/output"

	"github.com/spf13/cobra"
)

func NewCommand(rt *clienv.Runtime, service app.Service) *cobra.Command {
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
				struct {
					ConfigFile  string `json:"config_file"`
					AppImageDir string `json:"appimage_dir"`
					DesktopDir  string `json:"desktop_dir"`
					IconDir     string `json:"icon_dir"`
				}{
					ConfigFile:  result.ConfigFile,
					AppImageDir: result.AppImageDir,
					DesktopDir:  result.DesktopDir,
					IconDir:     result.IconDir,
				},
				func(w io.Writer) error {
					fmt.Fprintf(w, "Config file:  %s\n", result.ConfigFile)
					fmt.Fprintf(w, "AppImage dir: %s\n", result.AppImageDir)
					fmt.Fprintf(w, "Desktop dir:  %s\n", result.DesktopDir)
					fmt.Fprintf(w, "Icon dir:     %s\n", result.IconDir)
					return nil
				},
			)
		},
	}

	return cmd
}
