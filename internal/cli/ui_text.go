package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	sectionApp      = "App"
	sectionAppImage = "AppImage"
	sectionUpdates  = "Updates"
	sectionState    = "State"
)

func progressCheckAimUpdates() string {
	return "Checking for aim updates"
}

func progressUpgradeAim() string {
	return "Upgrading aim"
}

func warningNoEmbeddedSource() string {
	return "No embedded update source found in the current AppImage"
}

func formatPrompt(action, target string) string {
	return fmt.Sprintf("%s %s? [y/N]: ", action, target)
}

func colorize(enabled bool, code, value string) string {
	if !enabled {
		return value
	}

	return code + value + "\033[0m"
}

func printSuccess(cmd *cobra.Command, text string) {
	if runtimeOptionsFrom(cmd).Quiet || !shouldRenderLogs(cmd) {
		return
	}
	writeLogf(cmd, "%s\n", colorize(shouldColorStderr(cmd), "\033[0;32m", text))
}

func printWarning(cmd *cobra.Command, text string) {
	if !shouldRenderLogs(cmd) {
		return
	}
	writeLogf(cmd, "%s\n", colorize(shouldColorStderr(cmd), "\033[0;33m", text))
}

func printError(cmd *cobra.Command, text string) {
	writeLogf(cmd, "%s\n", colorize(shouldColorStderr(cmd), "\033[0;31m", text))
}

func printInfo(cmd *cobra.Command, text string) {
	if runtimeOptionsFrom(cmd).Quiet || !shouldRenderLogs(cmd) {
		return
	}
	writeLogf(cmd, "%s\n", colorize(shouldColorStderr(cmd), "\033[0;36m", text))
}

func printSection(cmd *cobra.Command, text string) {
	if shouldUseStructuredOutput(cmd) {
		return
	}
	writeDataf(cmd, "%s\n", colorize(shouldColorStdout(cmd), "\033[1m", text))
}

func printCurrentIncoming(cmd *cobra.Command, current, incoming string) {
	writeLogf(cmd, "Current:\n")
	writeLogf(cmd, "  %s\n", current)
	writeLogf(cmd, "Incoming:\n")
	writeLogf(cmd, "  %s\n", incoming)
}

func printCurrentValue(cmd *cobra.Command, current string) {
	writeLogf(cmd, "Current:\n")
	writeLogf(cmd, "  %s\n", current)
}
