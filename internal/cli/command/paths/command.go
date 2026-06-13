package paths

import (
	"fmt"
	"io"

	"aim/internal/app"
	"aim/internal/cli/clienv"
	"aim/internal/cli/output"

	"github.com/spf13/cobra"
)

func NewCommand(rt *clienv.Runtime, service app.PathProvider) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "paths",
		Short: "Show aim paths.",
		Long:  "Show the config, storage, cache, desktop entry, and icon paths used by aim.",
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
					CacheDir    string `json:"cache_dir"`
					DesktopDir  string `json:"desktop_dir"`
					IconDir     string `json:"icon_dir"`
				}{
					ConfigFile:  result.ConfigFile,
					AppImageDir: result.AppImageDir,
					CacheDir:    result.CacheDir,
					DesktopDir:  result.DesktopDir,
					IconDir:     result.IconDir,
				},
				func(w io.Writer) error {
					fmt.Fprintf(w, "Config file:  %s\n", result.ConfigFile)
					fmt.Fprintf(w, "AppImage dir: %s\n", result.AppImageDir)
					fmt.Fprintf(w, "Cache dir:    %s\n", result.CacheDir)
					fmt.Fprintf(w, "Desktop dir:  %s\n", result.DesktopDir)
					fmt.Fprintf(w, "Icon dir:     %s\n", result.IconDir)
					return nil
				},
			)
		},
	}

	return cmd
}
