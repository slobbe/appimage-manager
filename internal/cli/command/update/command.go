package update

import (
	"bufio"
	"context"
	"fmt"
	"io"
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

func NewCommand(rt *clienv.Runtime, service app.Service) *cobra.Command {
	var setID string
	var unsetID string
	var githubRepo string
	var assetPattern string
	var embedded bool
	var prerelease bool
	var checkOnly bool

	cmd := &cobra.Command{
		Use:     "update [appimage]",
		Aliases: []string{"u"},
		Short:   "Update integrated AppImages.",
		Long:    "Check integrated AppImages for updates and optionally update them.",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if checkOnly && (setID != "" || unsetID != "" || githubRepo != "" || assetPattern != "" || embedded || prerelease) {
				return fmt.Errorf("--check cannot be combined with update source flags")
			}
			if setID != "" || unsetID != "" || githubRepo != "" || assetPattern != "" || embedded || prerelease {
				return runUpdateSourceCommand(cmd, rt, service, updateSourceFlags{
					setID:        setID,
					unsetID:      unsetID,
					githubRepo:   githubRepo,
					assetPattern: assetPattern,
					embedded:     embedded,
					prerelease:   prerelease,
				}, args)
			}

			reporter := activity.NewReporter(cmd.ErrOrStderr(), !rt.Config.JSON)

			req := app.UpdateRequest{
				CheckOnly: checkOnly,
				Activity:  reporter,
				Confirmation: updatePrompter{
					in:          cmd.InOrStdin(),
					out:         cmd.OutOrStdout(),
					autoConfirm: rt.Config.JSON,
				},
			}
			if len(args) > 0 {
				req.Target = args[0]
			}

			result, err := service.Update(cmd.Context(), req)
			if err != nil {
				reporter.Wait()
				return err
			}
			reporter.Wait()

			return output.Write(
				cmd.OutOrStdout(),
				rt.Config.JSON,
				struct {
					Status  string                `json:"status"`
					Action  string                `json:"action"`
					Target  string                `json:"target,omitempty"`
					Applied bool                  `json:"applied"`
					Updates []app.UpdateCandidate `json:"updates"`
				}{
					Status:  "ok",
					Action:  "update",
					Target:  req.Target,
					Applied: result.Applied,
					Updates: result.Updates,
				},
				func(w io.Writer) error {
					if len(result.Updates) == 0 {
						fmt.Fprintln(w, "All apps up-to-date")
						return nil
					}
					if req.CheckOnly {
						fmt.Fprintln(w, "Updates available:")
						writeUpdateCandidates(w, result.Updates)
						return nil
					}
					if !result.Applied {
						fmt.Fprintln(w, "Update canceled")
						return nil
					}
					if req.Target != "" {
						fmt.Fprintf(w, "%sSuccessfully updated %s!%s\n", green, req.Target, reset)
						return nil
					}
					fmt.Fprintf(w, "%sSuccessfully updated all apps!%s\n", green, reset)
					return nil
				},
			)
		},
	}

	cmd.Flags().StringVar(&setID, "set", "", "set update source for app ID")
	cmd.Flags().StringVar(&unsetID, "unset", "", "unset update source for app ID")
	cmd.Flags().StringVar(&githubRepo, "github", "", "set GitHub update source in owner/repo format")
	cmd.Flags().StringVar(&assetPattern, "asset", "", "match the GitHub AppImage asset name using filepath.Match syntax")
	cmd.Flags().BoolVar(&embedded, "embedded", false, "set update source from embedded AppImage update information")
	cmd.Flags().BoolVar(&prerelease, "prerelease", false, "include prereleases for GitHub update source")
	cmd.Flags().BoolVar(&checkOnly, "check", false, "check for updates without applying them")

	return cmd
}

type updateSourceFlags struct {
	setID        string
	unsetID      string
	githubRepo   string
	assetPattern string
	embedded     bool
	prerelease   bool
}

func runUpdateSourceCommand(cmd *cobra.Command, rt *clienv.Runtime, service app.Service, flags updateSourceFlags, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("update source flags do not accept positional arguments")
	}
	if flags.setID != "" && flags.unsetID != "" {
		return fmt.Errorf("provide either --set or --unset, not both")
	}
	if flags.unsetID != "" {
		if flags.githubRepo != "" || flags.assetPattern != "" || flags.embedded || flags.prerelease {
			return fmt.Errorf("--unset cannot be combined with --github, --asset, --embedded, or --prerelease")
		}
		return unsetUpdateSource(cmd, rt, service, flags.unsetID)
	}
	if flags.setID == "" {
		return fmt.Errorf("--github, --asset, --embedded, and --prerelease require --set")
	}
	if flags.assetPattern != "" && flags.githubRepo == "" {
		return fmt.Errorf("--asset requires --github")
	}
	if flags.githubRepo != "" && flags.embedded {
		return fmt.Errorf("provide either --github or --embedded, not both")
	}
	if flags.githubRepo == "" && !flags.embedded {
		return fmt.Errorf("--set requires --github or --embedded")
	}
	if flags.embedded && flags.prerelease {
		return fmt.Errorf("--prerelease can only be used with --github")
	}

	return setUpdateSource(cmd, rt, service, flags)
}

func setUpdateSource(cmd *cobra.Command, rt *clienv.Runtime, service app.Service, flags updateSourceFlags) error {
	result, err := service.SetUpdateSource(cmd.Context(), app.SetUpdateSourceRequest{
		ID:           flags.setID,
		GitHubRepo:   flags.githubRepo,
		AssetPattern: flags.assetPattern,
		Prerelease:   flags.prerelease,
		Embedded:     flags.embedded,
	})
	if err != nil {
		return err
	}

	return output.Write(
		cmd.OutOrStdout(),
		rt.Config.JSON,
		struct {
			Status string `json:"status"`
			Action string `json:"action"`
			ID     string `json:"id"`
			Kind   string `json:"kind"`
		}{
			Status: "ok",
			Action: "set_update_source",
			ID:     result.ID,
			Kind:   string(result.UpdateSource.Kind),
		},
		func(w io.Writer) error {
			fmt.Fprintf(w, "%sSet update source for %s.%s\n", green, result.ID, reset)
			return nil
		},
	)
}

func unsetUpdateSource(cmd *cobra.Command, rt *clienv.Runtime, service app.Service, id string) error {
	if err := service.UnsetUpdateSource(cmd.Context(), app.UnsetUpdateSourceRequest{ID: id}); err != nil {
		return err
	}

	return output.Write(
		cmd.OutOrStdout(),
		rt.Config.JSON,
		struct {
			Status string `json:"status"`
			Action string `json:"action"`
			ID     string `json:"id"`
		}{
			Status: "ok",
			Action: "unset_update_source",
			ID:     id,
		},
		func(w io.Writer) error {
			fmt.Fprintf(w, "%sUnset update source for %s.%s\n", green, id, reset)
			return nil
		},
	)
}

type updatePrompter struct {
	in          io.Reader
	out         io.Writer
	autoConfirm bool
}

func (p updatePrompter) ConfirmUpdates(ctx context.Context, updates []app.UpdateCandidate) (bool, error) {
	if p.autoConfirm {
		return true, nil
	}

	if err := ctx.Err(); err != nil {
		return false, err
	}

	writeUpdateCandidates(p.out, updates)
	fmt.Fprintln(p.out)
	fmt.Fprint(p.out, "Update all apps? (y/n) ")

	reader := bufio.NewReader(p.in)
	answer, err := reader.ReadString('\n')
	fmt.Fprintln(p.out)
	if err != nil && len(answer) == 0 {
		return false, err
	}

	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes", nil
}

func writeUpdateCandidates(w io.Writer, updates []app.UpdateCandidate) {
	idWidth := 0
	versionWidth := 0
	for _, update := range updates {
		idWidth = max(idWidth, len(update.ID)+2)
		versionWidth = max(versionWidth, len(update.CurrentVersion))
	}

	for _, update := range updates {
		fmt.Fprintf(
			w,
			"%-*s %-*s -> %s\n",
			idWidth,
			"["+update.ID+"]",
			versionWidth,
			update.CurrentVersion,
			update.NewVersion,
		)
	}
}
