package main

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
