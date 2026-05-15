package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func printPrompt(cmd *cobra.Command, prompt string) {
	writeLogf(cmd, "%s", prompt)
}

func confirmAction(cmd *cobra.Command, prompt string) (bool, error) {
	opts := runtimeOptionsFrom(cmd)
	if opts.Yes {
		return true, nil
	}
	if opts.DryRun {
		return true, nil
	}
	if opts.NoInput {
		return false, noPermError(fmt.Errorf("confirmation required with --no-input; rerun with --yes to continue non-interactively"))
	}
	if !terminalInputChecker() {
		return false, noPermError(fmt.Errorf("confirmation required in non-interactive mode; rerun with --yes"))
	}

	printPrompt(cmd, prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return false, noPermError(err)
		}
		return false, softwareError(err)
	}

	answer := strings.TrimSpace(line)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes"), nil
}

func canPromptForInput(cmd *cobra.Command) bool {
	opts := runtimeOptionsFrom(cmd)
	return !opts.NoInput && terminalInputChecker()
}

func readPromptedValue(cmd *cobra.Command, prompt string) (string, error) {
	printPrompt(cmd, prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", softwareError(err)
	}
	return strings.TrimSpace(line), nil
}

func resolveSingleInputOrPrompt(cmd *cobra.Command, args []string, usage, prompt string, missingErr error) (string, error) {
	value, err := commandSingleArg(args, usage)
	if err == nil {
		return value, nil
	}
	if len(args) > 1 {
		return "", err
	}
	if !canPromptForInput(cmd) {
		if missingErr != nil && isMissingArgumentError(err) {
			return "", missingErr
		}
		return "", err
	}

	value, promptErr := readPromptedValue(cmd, prompt)
	if promptErr != nil {
		return "", promptErr
	}
	if strings.TrimSpace(value) == "" {
		if missingErr != nil {
			return "", missingErr
		}
		return "", usageError(fmt.Errorf("missing required argument %s", usage))
	}
	return value, nil
}

func isMissingArgumentError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(err.Error())), "missing required argument")
}

func missingInputErrorForAdd() error {
	return usageError(fmt.Errorf("missing required input; pass <id|Path/To.AppImage> or one of --url or --github"))
}

func missingInputErrorForInfo() error {
	return usageError(fmt.Errorf("missing required input; pass <id|Path/To.AppImage> or --github"))
}

func missingInputErrorForManagedID() error {
	return usageError(fmt.Errorf("missing required input; pass <id> as a positional argument"))
}

func missingInputErrorForRemove() error {
	return missingInputErrorForManagedID()
}
