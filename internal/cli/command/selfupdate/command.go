package selfupdate

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
	SelfUpdate(ctx context.Context, req app.SelfUpdateRequest) (app.SelfUpdateResult, error)
}

func NewCommand(rt *clienv.Runtime, service service) *cobra.Command {
	var prerelease bool

	cmd := &cobra.Command{
		Use:   "selfupdate",
		Short: "Update aim itself",
		Long:  "Update the aim CLI.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			reporter := activity.NewReporter(cmd.ErrOrStderr(), rt.ActivityEnabled(cmd.ErrOrStderr()))

			req := app.SelfUpdateRequest{
				Prerelease: prerelease,
				Activity:   reporter,
				Confirmation: selfUpdatePrompter{
					in:  cmd.InOrStdin(),
					out: confirmationOutput(cmd, rt.Config.JSON),
					options: prompt.ConfirmOptions{
						AutoConfirm:    rt.Config.Yes,
						NonInteractive: rt.Config.NonInteractive,
					},
				},
			}

			result, err := service.SelfUpdate(cmd.Context(), req)
			if err != nil {
				reporter.Wait()
				return err
			}
			reporter.Wait()

			return output.Write(
				cmd.OutOrStdout(),
				rt.Config.JSON,
				struct {
					Status  string `json:"status"`
					Action  string `json:"action"`
					Applied bool   `json:"applied"`
					From    string `json:"from"`
					To      string `json:"to"`
				}{
					Status:  selfUpdateStatus(result),
					Action:  "selfupdate",
					Applied: result.Applied,
					From:    result.Update.CurrentVersion,
					To:      result.Update.NewVersion,
				},
				func(w io.Writer) error {
					if !result.Applied {
						_, err := fmt.Fprintln(w, "Self-update canceled")
						return err
					}
					if result.Update.CurrentVersion == result.Update.NewVersion {
						_, err := fmt.Fprintln(w, rt.Success(w, fmt.Sprintf("aim is already up-to-date (%s).", result.Update.NewVersion)))
						return err
					}
					_, err := fmt.Fprintln(w, rt.Success(w, fmt.Sprintf("Successfully updated aim to %s!", result.Update.NewVersion)))
					return err
				},
			)
		},
	}

	cmd.Flags().BoolVar(&prerelease, "prerelease", false, "allow installing the latest prerelease")

	return cmd
}

func selfUpdateStatus(result app.SelfUpdateResult) string {
	if !result.Applied {
		return "canceled"
	}
	if result.Update.CurrentVersion == result.Update.NewVersion {
		return "up_to_date"
	}
	return "updated"
}

type selfUpdatePrompter struct {
	in      io.Reader
	out     io.Writer
	options prompt.ConfirmOptions
}

func (p selfUpdatePrompter) ConfirmSelfUpdate(ctx context.Context, update app.SelfUpdateCandidate) (bool, error) {
	question := fmt.Sprintf("Update aim from %s to %s? (y/n) ", update.CurrentVersion, update.NewVersion)
	return prompt.ConfirmYesNo(ctx, p.in, p.out, question, p.options)
}

func confirmationOutput(cmd *cobra.Command, jsonOutput bool) io.Writer {
	if jsonOutput {
		return cmd.ErrOrStderr()
	}
	return cmd.OutOrStdout()
}
