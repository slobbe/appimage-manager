package info

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
	Info(ctx context.Context, req app.InfoRequest) (app.InfoResult, error)
}

func NewCommand(rt *clienv.Runtime, service service) *cobra.Command {
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
					return writeInfo(w, result, rt)
				},
			)
		},
	}

	return cmd
}

func writeInfo(w io.Writer, result app.InfoResult, rt *clienv.Runtime) error {
	writer := &infoWriter{w: w}
	writer.line(rt.Emphasis(w, title(result)))
	writeInstallationStatus(writer, result)
	writer.field("Exec path:", result.ExecPath)
	writeSource(writer, result)
	writeUpdateSource(writer, result)
	return writer.err
}

type infoWriter struct {
	w   io.Writer
	err error
}

func (w *infoWriter) line(value string) {
	if w.err == nil {
		_, w.err = fmt.Fprintln(w.w, value)
	}
}

func (w *infoWriter) field(label string, value any) {
	if w.err == nil {
		_, w.err = fmt.Fprintf(w.w, "%-17s %v\n", label, value)
	}
}

func writeInstallationStatus(w *infoWriter, result app.InfoResult) {
	status := "not installed"
	if result.Installed {
		status = "installed"
	}
	w.field("Status:", status)
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

func writeSource(w *infoWriter, info app.InfoResult) {
	source := info.Source
	kind := string(source.Kind)
	if kind == "" {
		kind = "unknown"
	}
	w.field("Source:", kind)

	switch string(source.Kind) {
	case "local":
		w.field("Original file:", source.LocalFile.Path)
		if !source.LocalFile.IntegratedAt.IsZero() {
			w.field("Integrated at:", output.FormatSourceTime(source.LocalFile.IntegratedAt))
		}
	case "github":
		w.field("Repository:", source.GitHubRelease.Repo)
		w.field("Release tag:", source.GitHubRelease.Tag)
		w.field("Asset:", source.GitHubRelease.Asset)
		if !source.GitHubRelease.DownloadedAt.IsZero() {
			w.field("Downloaded at:", output.FormatSourceTime(source.GitHubRelease.DownloadedAt))
		}
	}
}

func writeUpdateSource(w *infoWriter, info app.InfoResult) {
	source := info.UpdateSource
	kind := string(source.Kind)
	if kind == "" {
		kind = "unknown"
	}
	w.field("Update source:", kind)
	w.field("Embedded update:", source.Embedded)
	if source.Raw != "" {
		w.field("Raw update:", source.Raw)
	}
	if source.Transport != "" {
		w.field("Transport:", source.Transport)
	}
	switch string(source.Kind) {
	case "github":
		w.field("Update repo:", source.Repo)
		if source.ReleaseTag != "" {
			w.field("Release tag:", source.ReleaseTag)
		}
		if source.AssetPattern != "" {
			w.field("Asset pattern:", source.AssetPattern)
		}
		if source.ZsyncAssetPattern != "" {
			w.field("Zsync pattern:", source.ZsyncAssetPattern)
		}
		w.field("Prereleases:", source.Prerelease)
	case "local_file":
		w.field("Update path:", source.Path)
		writePreservedUpdateSourceStatus(w)
	case "zsync":
		w.field("Zsync URL:", source.URL)
		writePreservedUpdateSourceStatus(w)
	case "unsupported":
		writePreservedUpdateSourceStatus(w)
	}
}

func writePreservedUpdateSourceStatus(w *infoWriter) {
	w.field("Update support:", "preserved; updates not applied by aim yet")
}
