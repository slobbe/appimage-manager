package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var manualLookPath = exec.LookPath
var manualCommandContext = exec.CommandContext
var manualPagerValue = func() string {
	return strings.TrimSpace(os.Getenv("PAGER"))
}
var manualOutputChecker = detectManualOutput

func newHelpCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:                "help [command]",
		Short:              "Show terminal documentation",
		Long:               "Show terminal documentation for aim or one of its commands.",
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runHelpCommand(cmd.Context(), root, args)
		},
	}
}

func maybeRunExplicitHelp(ctx context.Context, root *cobra.Command, args []string) (bool, error) {
	if root == nil || !hasHelpFlag(args) {
		return false, nil
	}

	target, err := resolveInlineHelpCommand(root, args)
	if err != nil {
		return true, err
	}
	root.SetContext(ctx)
	_, err = io.WriteString(root.OutOrStdout(), renderFullHelp(target))
	return true, err
}

func runHelpCommand(ctx context.Context, root *cobra.Command, args []string) error {
	target, err := resolveHelpTopicCommand(root, args)
	if err != nil {
		return err
	}

	roff, err := renderManPage(target, 1)
	if err != nil {
		return err
	}
	return displayManual(ctx, root, target, renderManualText(target), roff)
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if isHelpFlag(arg) {
			return true
		}
	}
	return false
}

func isHelpFlag(arg string) bool {
	return strings.TrimSpace(arg) == "--help" || strings.TrimSpace(arg) == "-h"
}

func resolveInlineHelpCommand(root *cobra.Command, args []string) (*cobra.Command, error) {
	if root == nil {
		return nil, usageError(fmt.Errorf("help is unavailable"))
	}
	if len(args) > 0 && isHelpFlag(args[0]) {
		return root, nil
	}
	return resolveCommandFromInvocation(root, args), nil
}

func resolveHelpTopicCommand(root *cobra.Command, args []string) (*cobra.Command, error) {
	if root == nil {
		return nil, usageError(fmt.Errorf("help is unavailable"))
	}

	filtered := nonFlagCommandTokens(args)
	if len(filtered) == 0 {
		return root, nil
	}

	current := root
	consumed := make([]string, 0, len(filtered))
	for _, token := range filtered {
		child := findSubcommandToken(current, token)
		if child == nil || child.Hidden {
			topic := strings.Join(append(consumed, token), " ")
			return nil, unknownHelpTopicError(root, strings.TrimSpace(topic))
		}
		current = child
		consumed = append(consumed, token)
	}

	return current, nil
}

func resolveCommandFromInvocation(root *cobra.Command, args []string) *cobra.Command {
	current := root
	stopCommands := false

	for idx := 0; idx < len(args); idx++ {
		token := strings.TrimSpace(args[idx])
		if token == "" || token == "--" {
			break
		}
		if isHelpFlag(token) {
			continue
		}
		if strings.HasPrefix(token, "--") {
			if consumesFlagValue(current, token) {
				idx++
			}
			continue
		}
		if strings.HasPrefix(token, "-") && token != "-" {
			if consumesShortFlagValue(current, token) {
				idx++
			}
			continue
		}
		if stopCommands {
			continue
		}
		child := findSubcommandToken(current, token)
		if child == nil {
			stopCommands = true
			continue
		}
		current = child
	}

	return current
}

func nonFlagCommandTokens(args []string) []string {
	filtered := make([]string, 0, len(args))
	for idx := 0; idx < len(args); idx++ {
		token := strings.TrimSpace(args[idx])
		if token == "" || token == "--" {
			break
		}
		if isHelpFlag(token) {
			continue
		}
		if strings.HasPrefix(token, "--") {
			if !strings.Contains(token, "=") && idx+1 < len(args) && !strings.HasPrefix(strings.TrimSpace(args[idx+1]), "-") {
				idx++
			}
			continue
		}
		if strings.HasPrefix(token, "-") && token != "-" {
			if idx+1 < len(args) && !strings.HasPrefix(strings.TrimSpace(args[idx+1]), "-") {
				idx++
			}
			continue
		}
		filtered = append(filtered, token)
	}
	return filtered
}

func consumesFlagValue(cmd *cobra.Command, token string) bool {
	name := strings.TrimSpace(strings.TrimPrefix(token, "--"))
	if name == "" || strings.Contains(name, "=") {
		return false
	}
	flag := lookupFlag(cmd, name)
	return flag != nil && !isBooleanFlag(flag)
}

func consumesShortFlagValue(cmd *cobra.Command, token string) bool {
	trimmed := strings.TrimSpace(strings.TrimPrefix(token, "-"))
	if trimmed == "" {
		return false
	}
	if len(trimmed) != 1 {
		return false
	}

	for _, flag := range collectVisibleFlags(cmd) {
		if flag.Shorthand == trimmed {
			return !isBooleanFlag(flag)
		}
	}
	return false
}

func findSubcommandToken(cmd *cobra.Command, token string) *cobra.Command {
	if cmd == nil {
		return nil
	}
	trimmed := strings.TrimSpace(token)
	for _, child := range cmd.Commands() {
		if child == nil {
			continue
		}
		if child.Name() == trimmed {
			return child
		}
		for _, alias := range child.Aliases {
			if strings.TrimSpace(alias) == trimmed {
				return child
			}
		}
	}
	return nil
}

func manPageNameForCommand(cmd *cobra.Command) string {
	if cmd == nil {
		return "aim"
	}
	path := strings.Fields(strings.TrimSpace(cmd.CommandPath()))
	if len(path) == 0 {
		return "aim"
	}
	return strings.Join(path, "-")
}

func displayManual(ctx context.Context, root, target *cobra.Command, plainText, roff string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !manualOutputChecker(root) {
		_, err := io.WriteString(root.OutOrStdout(), plainText)
		return err
	}
	if err := displayManualWithMan(ctx, root, target, roff); err == nil {
		return nil
	}
	if err := displayManualWithPager(ctx, root, target, plainText); err == nil {
		return nil
	}
	_, err := io.WriteString(root.OutOrStdout(), plainText)
	return err
}

func displayManualWithMan(ctx context.Context, root, target *cobra.Command, roff string) error {
	binary, err := manualLookPath("man")
	if err != nil {
		return err
	}

	tempFile, err := os.CreateTemp("", manPageNameForCommand(target)+"-*.1")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}()

	if _, err := tempFile.WriteString(roff); err != nil {
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}

	command := manualCommandContext(ctx, binary, "-l", tempPath)
	command.Stdin = os.Stdin
	command.Stdout = root.OutOrStdout()
	command.Stderr = root.ErrOrStderr()
	return command.Run()
}

func displayManualWithPager(ctx context.Context, root, target *cobra.Command, plainText string) error {
	pager := manualPagerValue()
	candidates := []string{}
	if pager != "" {
		candidates = append(candidates, strings.Fields(pager)...)
	}
	candidates = append(candidates, "less", "more")

	var binary string
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		found, err := manualLookPath(candidate)
		if err == nil {
			binary = found
			break
		}
	}
	if binary == "" {
		return exec.ErrNotFound
	}

	tempFile, err := os.CreateTemp("", manPageNameForCommand(target)+"-*.txt")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}()

	if _, err := tempFile.WriteString(plainText); err != nil {
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}

	command := manualCommandContext(ctx, binary, tempPath)
	command.Stdin = os.Stdin
	command.Stdout = root.OutOrStdout()
	command.Stderr = root.ErrOrStderr()
	return command.Run()
}

func detectManualOutput(root *cobra.Command) bool {
	if root == nil {
		return false
	}
	file, ok := root.OutOrStdout().(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func unknownHelpTopicError(root *cobra.Command, topic string) error {
	message := fmt.Sprintf("unknown help topic %q", topic)
	if suggestion := suggestedCommandName(root, topic); suggestion != "" {
		message += fmt.Sprintf("\nDid you mean %q?", suggestion)
	}
	return usageError(fmt.Errorf("%s", message))
}

func suggestedCommandName(root *cobra.Command, value string) string {
	if root == nil {
		return ""
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	suggestions := root.SuggestionsFor(value)
	if len(suggestions) == 0 {
		return ""
	}
	return strings.TrimSpace(suggestions[0])
}

func documentedCommands(root *cobra.Command) []*cobra.Command {
	commands := []*cobra.Command{root}
	for _, child := range collectVisibleCommands(root) {
		if child.Name() == "help" {
			continue
		}
		commands = append(commands, child)
	}
	return commands
}

func manPagePathForCommand(root *cobra.Command, cmd *cobra.Command) string {
	if root == nil {
		return filepath.Join("docs", "aim.1")
	}
	return filepath.Join("docs", manPageNameForCommand(cmd)+".1")
}
