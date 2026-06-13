package add

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"aim/internal/app"
	"aim/internal/cli/activity"
	"aim/internal/cli/clienv"
	"aim/internal/cli/output"

	"github.com/spf13/cobra"
)

const (
	green = "\033[32m"
	reset = "\033[0m"
)

func NewCommand(rt *clienv.Runtime, service app.Adder) *cobra.Command {
	var githubRepo string
	var assetPattern string
	var prerelease bool

	cmd := &cobra.Command{
		Use:     "add <appimage-path>",
		Aliases: []string{"a"},
		Short:   "Add an AppImage.",
		Long:    "Add a local AppImage or download and add an AppImage from a GitHub release.",
		Args: func(cmd *cobra.Command, args []string) error {
			if githubRepo != "" && len(args) > 0 {
				return fmt.Errorf("provide either <appimage-path> or --github, not both")
			}
			if assetPattern != "" && githubRepo == "" {
				return fmt.Errorf("--asset requires --github")
			}
			if githubRepo == "" && len(args) != 1 {
				return fmt.Errorf("requires exactly one appimage path unless --github is used")
			}
			if githubRepo != "" && !strings.Contains(githubRepo, "/") {
				return fmt.Errorf("--github must be in owner/repo format")
			}
			if prerelease && githubRepo == "" {
				return fmt.Errorf("--prerelease requires --github")
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			reporter := activity.NewReporter(cmd.ErrOrStderr(), !rt.Config.JSON)

			req := app.AddRequest{
				GitHubRepo:   githubRepo,
				AssetPattern: assetPattern,
				Prerelease:   prerelease,
				Activity:     reporter,
			}
			if len(args) == 1 {
				path, err := normalizeLocalAppImagePath(args[0])
				if err != nil {
					return err
				}
				req.Path = path
			}

			result, err := service.Add(cmd.Context(), req)
			if err != nil {
				reporter.Wait()
				return err
			}
			reporter.Wait()

			return output.Write(
				cmd.OutOrStdout(),
				rt.Config.JSON,
				struct {
					Status     string `json:"status"`
					Action     string `json:"action"`
					Path       string `json:"path,omitempty"`
					GitHubRepo string `json:"github_repo,omitempty"`
					Name       string `json:"name"`
					ID         string `json:"id"`
				}{
					Status:     "ok",
					Action:     "add",
					Path:       req.Path,
					GitHubRepo: req.GitHubRepo,
					Name:       result.App.Name,
					ID:         result.App.ID,
				},
				func(w io.Writer) error {
					fmt.Fprintf(w, "%sSuccessfully integrated %s [%s]!%s\n", green, result.App.Name, result.App.ID, reset)
					return nil
				},
			)
		},
	}

	cmd.Flags().StringVar(&githubRepo, "github", "", "download and add an AppImage from a GitHub repository in owner/repo format")
	cmd.Flags().StringVar(&assetPattern, "asset", "", "match the GitHub AppImage asset name using filepath.Match syntax")
	cmd.Flags().BoolVar(&prerelease, "prerelease", false, "include GitHub prereleases when adding from --github")

	return cmd
}

func normalizeLocalAppImagePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("appimage path is required")
	}

	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve appimage path %q: %w", path, err)
	}

	return absolutePath, nil
}
