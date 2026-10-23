package diagnose

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli/clienv"
	"github.com/slobbe/appimage-manager/internal/cli/output"

	"github.com/spf13/cobra"
)

type service interface {
	Diagnose(ctx context.Context, req app.DiagnoseRequest) (app.DiagnoseResult, error)
}

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	if e.Code == 1 {
		return "diagnosis found compatibility warnings"
	}
	return "diagnosis incomplete"
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

func NewCommand(rt *clienv.Runtime, service service) *cobra.Command {
	return &cobra.Command{
		Use:           "diagnose",
		Short:         "Check host compatibility for AppImages",
		Long:          "Run read-only host compatibility checks without launching or modifying AppImages.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := service.Diagnose(cmd.Context(), app.DiagnoseRequest{})
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}

			if err := output.Write(cmd.OutOrStdout(), rt.Config.JSON, result, func(w io.Writer) error {
				var text strings.Builder
				name := result.System.PrettyName
				if name == "" {
					name = result.System.ID
				}
				if name == "" {
					name = "unknown"
				}
				architecture := result.System.Architecture
				if architecture == "" {
					architecture = "unknown"
				}
				fmt.Fprintf(&text, "System: %s\nArchitecture: %s\n", name, architecture)
				if result.System.Error != "" {
					fmt.Fprintf(&text, "System error: %s\n", result.System.Error)
				}
				for _, finding := range result.Findings {
					fmt.Fprintf(&text, "\n%s: %s\n", strings.ToUpper(string(finding.Status)), finding.Summary)
					if finding.Evidence != "" {
						fmt.Fprintf(&text, "  Evidence: %s\n", finding.Evidence)
					}
					if finding.Guidance != "" {
						fmt.Fprintf(&text, "  Guidance: %s\n", finding.Guidance)
					}
					if finding.Command != "" {
						fmt.Fprintf(&text, "  Command: %s\n", finding.Command)
					}
					if finding.DocumentationURL != "" {
						fmt.Fprintf(&text, "  Documentation: %s\n", finding.DocumentationURL)
					}
				}
				text.WriteString("\nHost checks do not guarantee that an AppImage will launch.\n")
				_, err := io.WriteString(w, text.String())
				return err
			}); err != nil {
				return &ExitError{Code: 2, Err: err}
			}

			if code := result.ExitCode(); code != 0 {
				return &ExitError{Code: code}
			}
			return nil
		},
	}
}
