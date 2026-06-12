package info

import (
	"fmt"
	"io"
	"time"

	"aim/internal/app"
	"aim/internal/cli/clienv"
	"aim/internal/cli/output"

	"github.com/spf13/cobra"
)

const (
	bold  = "\033[1m"
	reset = "\033[0m"
)

func NewCommand(rt *clienv.Runtime, service app.Service) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <appimage-or-path>",
		Short: "Get information about an AppImage.",
		Long:  "Get information about an integrated AppImage or a local AppImage file.",
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
				infoJSON{
					ID:           result.ID,
					Name:         result.Name,
					Version:      result.Version,
					ExecPath:     result.ExecPath,
					Source:       sourceToJSON(result),
					UpdateSource: updateSourceToJSON(result),
				},
				func(w io.Writer) error {
					writeInfo(w, result)
					return nil
				},
			)
		},
	}

	return cmd
}

type infoJSON struct {
	ID           string           `json:"id,omitempty"`
	Name         string           `json:"name"`
	Version      string           `json:"version"`
	ExecPath     string           `json:"exec_path"`
	Source       sourceJSON       `json:"source"`
	UpdateSource updateSourceJSON `json:"update_source"`
}

type updateSourceJSON struct {
	Embedded          bool   `json:"embedded"`
	Kind              string `json:"kind"`
	Raw               string `json:"raw,omitempty"`
	Transport         string `json:"transport,omitempty"`
	Repo              string `json:"repo,omitempty"`
	Path              string `json:"path,omitempty"`
	Prerelease        bool   `json:"prerelease,omitempty"`
	ReleaseTag        string `json:"release_tag,omitempty"`
	AssetPattern      string `json:"asset_pattern,omitempty"`
	ZsyncAssetPattern string `json:"zsync_asset_pattern,omitempty"`
	URL               string `json:"url,omitempty"`
}

type sourceJSON struct {
	Kind          string                   `json:"kind"`
	LocalFile     *localFileSourceJSON     `json:"local_file,omitempty"`
	GitHubRelease *githubReleaseSourceJSON `json:"github_release,omitempty"`
}

type localFileSourceJSON struct {
	Path         string `json:"path"`
	IntegratedAt string `json:"integrated_at,omitempty"`
}

type githubReleaseSourceJSON struct {
	Repo         string `json:"repo"`
	Tag          string `json:"tag,omitempty"`
	Asset        string `json:"asset,omitempty"`
	DownloadURL  string `json:"download_url,omitempty"`
	SizeBytes    int64  `json:"size_bytes,omitempty"`
	DownloadedAt string `json:"downloaded_at,omitempty"`
}

func updateSourceToJSON(info app.InfoResult) updateSourceJSON {
	source := info.UpdateSource
	return updateSourceJSON{
		Embedded:          source.Embedded,
		Kind:              string(source.Kind),
		Raw:               source.Raw,
		Transport:         source.Transport,
		Repo:              source.Repo,
		Path:              source.Path,
		Prerelease:        source.Prerelease,
		ReleaseTag:        source.ReleaseTag,
		AssetPattern:      source.AssetPattern,
		ZsyncAssetPattern: source.ZsyncAssetPattern,
		URL:               source.URL,
	}
}

func sourceToJSON(info app.InfoResult) sourceJSON {
	source := info.Source
	result := sourceJSON{Kind: string(source.Kind)}
	switch string(source.Kind) {
	case "local":
		result.LocalFile = &localFileSourceJSON{
			Path:         source.LocalFile.Path,
			IntegratedAt: formatSourceTime(source.LocalFile.IntegratedAt),
		}
	case "github":
		result.GitHubRelease = &githubReleaseSourceJSON{
			Repo:         source.GitHubRelease.Repo,
			Tag:          source.GitHubRelease.Tag,
			Asset:        source.GitHubRelease.Asset,
			DownloadURL:  source.GitHubRelease.DownloadURL,
			SizeBytes:    source.GitHubRelease.SizeBytes,
			DownloadedAt: formatSourceTime(source.GitHubRelease.DownloadedAt),
		}
	}

	return result
}

func writeInfo(w io.Writer, result app.InfoResult) {
	fmt.Fprintf(w, "%s%s%s\n", bold, title(result), reset)
	fmt.Fprintf(w, "%-17s %s\n", "Exec path:", result.ExecPath)
	writeSource(w, result)
	writeUpdateSource(w, result)
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
			fmt.Fprintf(w, "%-17s %s\n", "Integrated at:", formatSourceTime(source.LocalFile.IntegratedAt))
		}
	case "github":
		fmt.Fprintf(w, "%-17s %s\n", "Repository:", source.GitHubRelease.Repo)
		fmt.Fprintf(w, "%-17s %s\n", "Release tag:", source.GitHubRelease.Tag)
		fmt.Fprintf(w, "%-17s %s\n", "Asset:", source.GitHubRelease.Asset)
		if !source.GitHubRelease.DownloadedAt.IsZero() {
			fmt.Fprintf(w, "%-17s %s\n", "Downloaded at:", formatSourceTime(source.GitHubRelease.DownloadedAt))
		}
	}
}

func formatSourceTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}

	return value.UTC().Format(time.RFC3339)
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
	case "zsync":
		fmt.Fprintf(w, "%-17s %s\n", "Zsync URL:", source.URL)
	}
}
