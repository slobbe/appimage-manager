package id

import (
	"fmt"
	"io"
	"strings"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/activity"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
	"github.com/slobbe/appimage-manager/internal/cli/output"

	"github.com/spf13/cobra"
)

const (
	green = "\033[32m"
	reset = "\033[0m"
)

func NewCommand(rt *clienv.Runtime, service app.IDManager) *cobra.Command {
	var newID string
	var auto bool

	cmd := &cobra.Command{
		Use:   "id <old-id> (--set <new-id> | --auto)",
		Short: "Manage an integrated app ID.",
		Long:  "Manage the stable ID of an integrated app. Use --set to choose an ID, or --auto to derive it from the installed AppImage metadata.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("requires exactly one app id")
			}
			if strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("app id is required")
			}
			if auto && strings.TrimSpace(newID) != "" {
				return fmt.Errorf("provide either --set or --auto, not both")
			}
			if !auto && strings.TrimSpace(newID) == "" {
				return fmt.Errorf("requires --set or --auto")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			reporter := activity.NewReporter(cmd.ErrOrStderr(), !rt.Config.JSON)
			result, err := service.SetID(cmd.Context(), app.SetIDRequest{
				CurrentID: args[0],
				NewID:     newID,
				Auto:      auto,
				Activity:  reporter,
			})
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
					PreviousID string `json:"previous_id"`
					ID         string `json:"id"`
					Changed    bool   `json:"changed"`
				}{
					Status:     "ok",
					Action:     "set_id",
					PreviousID: result.PreviousID,
					ID:         result.ID,
					Changed:    result.Changed,
				},
				func(w io.Writer) error {
					if !result.Changed {
						fmt.Fprintf(w, "%sApp ID already %s.%s\n", green, result.ID, reset)
						return nil
					}
					fmt.Fprintf(w, "%sChanged app ID from %s to %s.%s\n", green, result.PreviousID, result.ID, reset)
					return nil
				},
			)
		},
	}

	cmd.Flags().StringVar(&newID, "set", "", "set the app ID to this value")
	cmd.Flags().BoolVar(&auto, "auto", false, "derive the app ID from installed AppImage metadata")

	return cmd
}
