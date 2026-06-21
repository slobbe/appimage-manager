package info

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
		Use:   "info <app-id|path>",
		Short: "Get information about an AppImage",
		Long:  "Get information about an integrated AppImage by app ID or inspect a local AppImage file. Local inspection extracts AppImage metadata by executing the AppImage's extraction/update-info modes; inspect only AppImages you trust.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := app.InfoRequest{
				Target: args[0],
			}

			result, err := service.Info(cmd.Context(), req)
			if err != nil {
				return err
			}

			return output.Write(
				cmd.OutOrStdout(),
				rt.Config.JSON,
				output.InfoResultJSON(result),
				func(w io.Writer) error {
					writeInfo(w, result)
					return nil
				},
			)
		},
	}

	return cmd
}

func writeInfo(w io.Writer, result app.InfoResult) {
	fmt.Fprintf(w, "%s%s%s\n", bold, title(result), reset)
	writeInstallationStatus(w, result)
	fmt.Fprintf(w, "%-17s %s\n", "Exec path:", result.ExecPath)
	writeSource(w, result)
	writeUpdateSource(w, result)
}

func writeInstallationStatus(w io.Writer, result app.InfoResult) {
	status := "not installed"
	if result.Installed {
		status = "installed"
	}
	fmt.Fprintf(w, "%-17s %s\n", "Status:", status)
}

func title(result app.InfoResult) string {
	version := result.Version
	if version == "" {
		version = "unknown"
	}

	if result.ID == "" {
		return fmt.Sprintf("%s (v%s)", result.Name, version)
	}

	return fmt.Sprintf("[%s] %s (v%s)", result.ID, result.Name, version)
}

func writeSource(w io.Writer, info app.InfoResult) {
	source := info.Source
	kind := string(source.Kind)
	if kind == "" {
		kind = "unknown"
	}
	fmt.Fprintf(w, "%-17s %s\n", "Source:", kind)

	switch string(source.Kind) {
	case "local":
		fmt.Fprintf(w, "%-17s %s\n", "Original file:", source.LocalFile.Path)
		if !source.LocalFile.IntegratedAt.IsZero() {
			fmt.Fprintf(w, "%-17s %s\n", "Integrated at:", output.FormatSourceTime(source.LocalFile.IntegratedAt))
		}
	case "github":
		fmt.Fprintf(w, "%-17s %s\n", "Repository:", source.GitHubRelease.Repo)
		fmt.Fprintf(w, "%-17s %s\n", "Release tag:", source.GitHubRelease.Tag)
		fmt.Fprintf(w, "%-17s %s\n", "Asset:", source.GitHubRelease.Asset)
		if !source.GitHubRelease.DownloadedAt.IsZero() {
			fmt.Fprintf(w, "%-17s %s\n", "Downloaded at:", output.FormatSourceTime(source.GitHubRelease.DownloadedAt))
		}
	}
}

func writeUpdateSource(w io.Writer, info app.InfoResult) {
	source := info.UpdateSource
	kind := string(source.Kind)
	if kind == "" {
		kind = "unknown"
	}
	fmt.Fprintf(w, "%-17s %s\n", "Update source:", kind)
	fmt.Fprintf(w, "%-17s %t\n", "Embedded update:", source.Embedded)
	if source.Raw != "" {
		fmt.Fprintf(w, "%-17s %s\n", "Raw update:", source.Raw)
	}
	if source.Transport != "" {
		fmt.Fprintf(w, "%-17s %s\n", "Transport:", source.Transport)
	}
	switch string(source.Kind) {
	case "github":
		fmt.Fprintf(w, "%-17s %s\n", "Update repo:", source.Repo)
		if source.ReleaseTag != "" {
			fmt.Fprintf(w, "%-17s %s\n", "Release tag:", source.ReleaseTag)
		}
		if source.AssetPattern != "" {
			fmt.Fprintf(w, "%-17s %s\n", "Asset pattern:", source.AssetPattern)
		}
		if source.ZsyncAssetPattern != "" {
			fmt.Fprintf(w, "%-17s %s\n", "Zsync pattern:", source.ZsyncAssetPattern)
		}
		fmt.Fprintf(w, "%-17s %t\n", "Prereleases:", source.Prerelease)
	case "local_file":
		fmt.Fprintf(w, "%-17s %s\n", "Update path:", source.Path)
		writePreservedUpdateSourceStatus(w)
	case "zsync":
		fmt.Fprintf(w, "%-17s %s\n", "Zsync URL:", source.URL)
		writePreservedUpdateSourceStatus(w)
	case "unsupported":
		writePreservedUpdateSourceStatus(w)
	}
}

func writePreservedUpdateSourceStatus(w io.Writer) {
	fmt.Fprintf(w, "%-17s %s\n", "Update support:", "preserved; updates not applied by aim yet")
}
