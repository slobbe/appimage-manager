package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

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
	return usageError(fmt.Errorf("missing required input; pass <id|Path/To.AppImage> or one of --url, --github, --gitlab"))
}

func missingInputErrorForInfo() error {
	return usageError(fmt.Errorf("missing required input; pass <id|Path/To.AppImage> or one of --github, --gitlab"))
}

func missingInputErrorForRemove() error {
	return usageError(fmt.Errorf("missing required input; pass <id> as a positional argument"))
}

func missingInputErrorForUpdateSet() error {
	return usageError(fmt.Errorf("missing required input; pass <id> as a positional argument"))
}

func missingInputErrorForUpdateUnset() error {
	return usageError(fmt.Errorf("missing required input; pass <id> as a positional argument"))
}
