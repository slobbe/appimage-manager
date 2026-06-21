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

const (
	green = "\033[32m"
	reset = "\033[0m"
)

func NewCommand(rt *clienv.Runtime, service app.Service) *cobra.Command {
	var prerelease bool

	cmd := &cobra.Command{
		Use:   "selfupdate",
		Short: "Update aim itself",
		Long:  "Update the aim CLI.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			reporter := activity.NewReporter(cmd.ErrOrStderr(), !rt.Config.JSON)

			req := app.SelfUpdateRequest{
				Prerelease: prerelease,
				Activity:   reporter,
				Confirmation: selfUpdatePrompter{
					in:          cmd.InOrStdin(),
					out:         cmd.OutOrStdout(),
					autoConfirm: rt.Config.JSON,
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
					Status:  "ok",
					Action:  "selfupdate",
					Applied: result.Applied,
					From:    result.Update.CurrentVersion,
					To:      result.Update.NewVersion,
				},
				func(w io.Writer) error {
					if !result.Applied {
						fmt.Fprintln(w, "Self-update canceled")
						return nil
					}
					if result.Update.CurrentVersion == result.Update.NewVersion {
						fmt.Fprintf(w, "%saim is already up-to-date (%s).%s\n", green, result.Update.NewVersion, reset)
						return nil
					}
					fmt.Fprintf(w, "%sSuccessfully updated aim to %s!%s\n", green, result.Update.NewVersion, reset)
					return nil
				},
			)
		},
	}

	cmd.Flags().BoolVar(&prerelease, "prerelease", false, "allow installing the latest prerelease")

	return cmd
}

type selfUpdatePrompter struct {
	in          io.Reader
	out         io.Writer
	autoConfirm bool
}

func (p selfUpdatePrompter) ConfirmSelfUpdate(ctx context.Context, update app.SelfUpdateCandidate) (bool, error) {
	question := fmt.Sprintf("Update aim from %s to %s? (y/n) ", update.CurrentVersion, update.NewVersion)
	return prompt.ConfirmYesNo(ctx, p.in, p.out, question, p.autoConfirm)
}
