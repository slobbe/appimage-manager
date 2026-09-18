package update

import (
	"context"
	"fmt"
	"io"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/activity"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
	"github.com/slobbe/appimage-manager/internal/cli/output"
	"github.com/slobbe/appimage-manager/internal/cli/prompt"

	"github.com/spf13/cobra"
)

type service interface {
	Update(ctx context.Context, req app.UpdateRequest) (app.UpdateResult, error)
	SetUpdateSource(ctx context.Context, req app.SetUpdateSourceRequest) (app.SetUpdateSourceResult, error)
	UnsetUpdateSource(ctx context.Context, req app.UnsetUpdateSourceRequest) error
}

func NewCommand(rt *clienv.Runtime, service service) *cobra.Command {
	var setID string
	var unsetID string
	var githubRepo string
	var assetPattern string
	var embedded bool
	var prerelease bool
	var checkOnly bool

	cmd := &cobra.Command{
		Use:           "update [appimage]",
		Aliases:       []string{"u"},
		Short:         "Update integrated AppImages",
		Long:          "Check integrated AppImages for updates and optionally update them.",
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if checkOnly && (setID != "" || unsetID != "" || githubRepo != "" || assetPattern != "" || embedded || prerelease) {
				return clienv.UsageError(fmt.Errorf("--check cannot be combined with update source flags"))
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

			reporter := activity.NewReporter(cmd.ErrOrStderr(), rt.ActivityEnabled(cmd.ErrOrStderr()))

			req := app.UpdateRequest{
				CheckOnly: checkOnly,
				Activity:  reporter,
				Confirmation: updatePrompter{
					in:  cmd.InOrStdin(),
					out: confirmationOutput(cmd, rt.Config.JSON),
					options: prompt.ConfirmOptions{
						AutoConfirm:    rt.Config.Yes,
						NonInteractive: rt.Config.NonInteractive,
					},
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
			if !rt.Config.JSON {
				if err := writeUpdateFailures(cmd.ErrOrStderr(), result.Failures); err != nil {
					return err
				}
				if err := writeUpdateSkips(cmd.ErrOrStderr(), result.Skipped); err != nil {
					return err
				}
				if err := output.WriteWarnings(cmd.ErrOrStderr(), result.Warnings); err != nil {
					return err
				}
			}

			status := updateStatus(req, result)
			updated := updatedCount(result)
			err = output.Write(
				cmd.OutOrStdout(),
				rt.Config.JSON,
				struct {
					Status    string                 `json:"status"`
					Action    string                 `json:"action"`
					Target    string                 `json:"target,omitempty"`
					Applied   bool                   `json:"applied"`
					Checked   int                    `json:"checked"`
					Available int                    `json:"available"`
					Updated   int                    `json:"updated"`
					Failed    int                    `json:"failed"`
					Skipped   []app.UpdateSkip       `json:"skipped"`
					Updates   []app.UpdateCandidate  `json:"updates"`
					Failures  []app.UpdateFailure    `json:"failures"`
					Warnings  []app.OperationWarning `json:"warnings"`
				}{
					Status:    status,
					Action:    "update",
					Target:    req.Target,
					Applied:   result.Applied,
					Checked:   result.Checked,
					Available: len(result.Updates),
					Updated:   updated,
					Failed:    len(result.Failures),
					Skipped:   itemsOrEmpty(result.Skipped),
					Updates:   itemsOrEmpty(result.Updates),
					Failures:  itemsOrEmpty(result.Failures),
					Warnings:  itemsOrEmpty(result.Warnings),
				},
				func(w io.Writer) error {
					if req.CheckOnly {
						if len(result.Updates) > 0 {
							if _, err := fmt.Fprintln(w, "Updates available:"); err != nil {
								return err
							}
							if err := writeUpdateCandidates(w, result.Updates); err != nil {
								return err
							}
							if len(result.Failures) > 0 {
								_, err := fmt.Fprintf(w, "%d app update check(s) failed.\n", len(result.Failures))
								return err
							}
							return nil
						}
						if len(result.Failures) > 0 {
							_, err := fmt.Fprintf(w, "No updates found for apps checked successfully; %d check(s) failed.\n", len(result.Failures))
							return err
						}
					}
					if len(result.Failures) > 0 {
						if updated > 0 {
							_, err := fmt.Fprintf(w, "Updated %d app(s); %d failed.\n", updated, len(result.Failures))
							return err
						}
						_, err := fmt.Fprintf(w, "No updates were applied; %d app(s) failed.\n", len(result.Failures))
						return err
					}
					if len(result.Updates) == 0 {
						if len(result.Skipped) > 0 {
							_, err := fmt.Fprintf(w, "No supported update source for %d app(s).\n", len(result.Skipped))
							return err
						}
						_, err := fmt.Fprintln(w, "All apps up-to-date")
						return err
					}
					if !result.Applied {
						_, err := fmt.Fprintln(w, "Update canceled")
						return err
					}
					if req.Target != "" {
						if len(result.Warnings) > 0 {
							_, err := fmt.Fprintln(w, rt.Success(w, fmt.Sprintf("Updated %s with warnings.", req.Target)))
							return err
						}
						_, err := fmt.Fprintln(w, rt.Success(w, fmt.Sprintf("Successfully updated %s!", req.Target)))
						return err
					}
					if len(result.Warnings) > 0 {
						_, err := fmt.Fprintln(w, rt.Success(w, fmt.Sprintf("Updated %d app(s) with warnings.", updated)))
						return err
					}
					_, err := fmt.Fprintln(w, rt.Success(w, "Successfully updated all apps!"))
					return err
				},
			)
			if err != nil {
				return err
			}
			if len(result.Failures) > 0 {
				return clienv.SilentFailure()
			}
			return nil
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

func runUpdateSourceCommand(cmd *cobra.Command, rt *clienv.Runtime, service service, flags updateSourceFlags, args []string) error {
	if len(args) > 0 {
		return clienv.UsageError(fmt.Errorf("update source flags do not accept positional arguments"))
	}
	if flags.setID != "" && flags.unsetID != "" {
		return clienv.UsageError(fmt.Errorf("provide either --set or --unset, not both"))
	}
	if flags.unsetID != "" {
		if flags.githubRepo != "" || flags.assetPattern != "" || flags.embedded || flags.prerelease {
			return clienv.UsageError(fmt.Errorf("--unset cannot be combined with --github, --asset, --embedded, or --prerelease"))
		}
		return unsetUpdateSource(cmd, rt, service, flags.unsetID)
	}
	if flags.setID == "" {
		return clienv.UsageError(fmt.Errorf("--github, --asset, --embedded, and --prerelease require --set"))
	}
	if flags.assetPattern != "" && flags.githubRepo == "" {
		return clienv.UsageError(fmt.Errorf("--asset requires --github"))
	}
	if flags.githubRepo != "" && flags.embedded {
		return clienv.UsageError(fmt.Errorf("provide either --github or --embedded, not both"))
	}
	if flags.githubRepo == "" && !flags.embedded {
		return clienv.UsageError(fmt.Errorf("--set requires --github or --embedded"))
	}
	if flags.embedded && flags.prerelease {
		return clienv.UsageError(fmt.Errorf("--prerelease can only be used with --github"))
	}

	return setUpdateSource(cmd, rt, service, flags)
}

func setUpdateSource(cmd *cobra.Command, rt *clienv.Runtime, service service, flags updateSourceFlags) error {
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
			_, err := fmt.Fprintln(w, rt.Success(w, fmt.Sprintf("Set update source for %s.", result.ID)))
			return err
		},
	)
}

func unsetUpdateSource(cmd *cobra.Command, rt *clienv.Runtime, service service, id string) error {
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
			_, err := fmt.Fprintln(w, rt.Success(w, fmt.Sprintf("Unset update source for %s.", id)))
			return err
		},
	)
}

type updatePrompter struct {
	in      io.Reader
	out     io.Writer
	options prompt.ConfirmOptions
}

func (p updatePrompter) ConfirmUpdates(ctx context.Context, updates []app.UpdateCandidate) (bool, error) {
	if p.options.RequiresInput() {
		if err := writeUpdateCandidates(p.out, updates); err != nil {
			return false, err
		}
		if _, err := fmt.Fprintln(p.out); err != nil {
			return false, err
		}
	}
	return prompt.ConfirmYesNo(ctx, p.in, p.out, "Update all apps? (y/n) ", p.options)
}

func confirmationOutput(cmd *cobra.Command, jsonOutput bool) io.Writer {
	if jsonOutput {
		return cmd.ErrOrStderr()
	}
	return cmd.OutOrStdout()
}

func writeUpdateFailures(w io.Writer, failures []app.UpdateFailure) error {
	for _, failure := range failures {
		if _, err := fmt.Fprintf(w, "Update error [%s]: %s\n", failure.AppID, failure.Error); err != nil {
			return err
		}
	}
	return nil
}

func writeUpdateSkips(w io.Writer, skipped []app.UpdateSkip) error {
	for _, skip := range skipped {
		if skip.SourceKind == "" {
			if _, err := fmt.Fprintf(w, "Skipped [%s]: %s\n", skip.AppID, skip.Reason); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(w, "Skipped [%s]: %s (%s)\n", skip.AppID, skip.Reason, skip.SourceKind); err != nil {
			return err
		}
	}
	return nil
}

func updateStatus(req app.UpdateRequest, result app.UpdateResult) string {
	if len(result.Failures) > 0 {
		if result.Checked > len(result.Failures) || updatedCount(result) > 0 || len(result.Skipped) > 0 {
			return "partial_failure"
		}
		return "failed"
	}
	if !req.CheckOnly && !result.Applied && len(result.Updates) > 0 {
		return "canceled"
	}
	if req.CheckOnly && len(result.Updates) > 0 {
		return "updates_available"
	}
	if updatedCount(result) > 0 {
		if len(result.Warnings) > 0 {
			return "updated_with_warnings"
		}
		return "updated"
	}
	if result.Checked > 0 && result.Checked == len(result.Skipped) {
		return "skipped"
	}
	return "up_to_date"
}

func updatedCount(result app.UpdateResult) int {
	if result.AppliedCount > 0 {
		return result.AppliedCount
	}
	if result.Applied && len(result.Failures) == 0 {
		return len(result.Updates)
	}
	return 0
}

func itemsOrEmpty[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func writeUpdateCandidates(w io.Writer, updates []app.UpdateCandidate) error {
	idWidth := 0
	versionWidth := 0
	for _, update := range updates {
		idWidth = max(idWidth, len(update.ID)+2)
		versionWidth = max(versionWidth, len(update.CurrentVersion))
	}

	for _, update := range updates {
		if _, err := fmt.Fprintf(
			w,
			"%-*s %-*s -> %s\n",
			idWidth,
			"["+update.ID+"]",
			versionWidth,
			update.CurrentVersion,
			update.NewVersion,
		); err != nil {
			return err
		}
	}
	return nil
}
