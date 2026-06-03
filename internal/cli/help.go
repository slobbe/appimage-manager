package cli

import (
	"bytes"
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/spf13/pflag"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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

const rootCommandDocsURL = "https://github.com/slobbe/appimage-manager#readme"

func installHelpSystem(root *cobra.Command) {
	if root == nil {
		return
	}

	root.SetHelpCommand(newHelpCommand(root))
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		_ = args
		writeDataf(cmd, "%s", renderFullHelp(cmd))
	})
}

func renderConciseHelp(cmd *cobra.Command) string {
	var sections []string
	sections = append(sections, renderDescriptionSection(cmd))
	sections = append(sections, renderUsageSection(cmd))
	if examples := renderExamplesSection(cmd); examples != "" {
		sections = append(sections, examples)
	}
	if commands := renderCommonCommandsSection(cmd); commands != "" {
		sections = append(sections, commands)
	}
	if flags := renderCommonFlagsSection(cmd); flags != "" {
		sections = append(sections, flags)
	}
	if commandName(cmd) == "aim" {
		sections = append(sections, renderRootUpdateModesSection())
		sections = append(sections, renderSupportSection(cmd))
	}
	sections = append(sections, renderConciseMoreInfoSection(cmd))
	return strings.TrimSpace(strings.Join(filterEmptySections(sections), "\n\n")) + "\n"
}

func printConciseHelpError(cmd *cobra.Command, message string) error {
	err := usageError(fmt.Errorf("%s", strings.TrimSpace(message)))
	writer := logWriter(cmd)
	_, _ = fmt.Fprintln(writer, err)
	_, _ = io.WriteString(writer, renderConciseHelp(cmd))
	return markErrorDisplayed(err)
}

func renderFullHelp(cmd *cobra.Command) string {
	var sections []string
	sections = append(sections, renderDescriptionSection(cmd))
	sections = append(sections, renderUsageSection(cmd))
	if examples := renderExamplesSection(cmd); examples != "" {
		sections = append(sections, examples)
	}
	if commands := renderCommonCommandsSection(cmd); commands != "" {
		sections = append(sections, commands)
	}
	if flags := renderCommonFlagsSection(cmd); flags != "" {
		sections = append(sections, flags)
	}
	if commands := renderAllCommandsSection(cmd); commands != "" {
		sections = append(sections, commands)
	}
	if flags := renderAllFlagsSection(cmd); flags != "" {
		sections = append(sections, flags)
	}
	if commandName(cmd) == "aim" {
		sections = append(sections, renderRootUpdateModesSection())
		sections = append(sections, renderSupportSection(cmd))
	}
	sections = append(sections, renderFullMoreInfoSection(cmd))
	return strings.TrimSpace(strings.Join(filterEmptySections(sections), "\n\n")) + "\n"
}

func renderManualText(cmd *cobra.Command) string {
	name := commandName(cmd)
	if name == "" {
		name = "aim"
	}

	var sections []string
	sections = append(sections, renderNamedBlock("NAME", []string{
		fmt.Sprintf("%s - %s", commandPathWithUse(cmd), renderDescription(cmd)),
	}))
	sections = append(sections, renderNamedBlock("SYNOPSIS", usageLinesForCommand(cmd)))
	sections = append(sections, renderNamedBlock("DESCRIPTION", []string{
		renderDescription(cmd),
	}))
	if examples := renderManualExamplesSection(cmd); examples != "" {
		sections = append(sections, examples)
	}
	if options := renderManualOptionsSection(cmd); options != "" {
		sections = append(sections, options)
	}
	if commands := renderManualCommandsSection(cmd); commands != "" {
		sections = append(sections, commands)
	}
	if name == "aim" {
		sections = append(sections, renderNamedBlock("EXIT STATUS", strings.Split(formatExitStatusSection(), "\n")))
		sections = append(sections, renderRootUpdateModesSection())
		sections = append(sections, renderSupportSection(cmd))
	}
	sections = append(sections, renderManualMoreInfoSection(cmd))
	return strings.TrimSpace(strings.Join(filterEmptySections(sections), "\n\n")) + "\n"
}

func renderDescriptionSection(cmd *cobra.Command) string {
	return renderDescription(cmd)
}

func renderDescription(cmd *cobra.Command) string {
	if cmd == nil {
		return rootCommandDescription
	}
	if long := strings.TrimSpace(cmd.Long); long != "" {
		return long
	}
	if short := strings.TrimSpace(cmd.Short); short != "" {
		return short
	}
	return rootCommandDescription
}

func renderUsageSection(cmd *cobra.Command) string {
	return renderNamedBlock("USAGE", usageLinesForCommand(cmd))
}

func renderExamplesSection(cmd *cobra.Command) string {
	lines := exampleLines(cmd)
	if len(lines) == 0 {
		return ""
	}
	return renderNamedBlock("EXAMPLES", lines)
}

func renderCommonCommandsSection(cmd *cobra.Command) string {
	if commandName(cmd) != "aim" {
		if len(visibleSubcommands(cmd)) == 0 {
			return ""
		}
		return renderNamedBlock("COMMANDS", formatCommandLines(orderedVisibleSubcommands(cmd)))
	}

	commands := commonRootCommands(cmd)
	if len(commands) == 0 {
		return ""
	}
	return renderNamedBlock("COMMON COMMANDS", formatCommandLines(commands))
}

func renderAllCommandsSection(cmd *cobra.Command) string {
	commands := orderedVisibleSubcommands(cmd)
	if len(commands) == 0 {
		return ""
	}

	title := "ALL COMMANDS"
	if commandName(cmd) != "aim" {
		title = "COMMANDS"
	}
	return renderNamedBlock(title, formatCommandLines(commands))
}

func renderCommonFlagsSection(cmd *cobra.Command) string {
	flags := flagsForNames(cmd, commonFlagNamesForCommand(cmd))
	if len(flags) == 0 {
		return ""
	}
	return renderNamedBlock("COMMON FLAGS", formatFlagLines(flags))
}

func renderAllFlagsSection(cmd *cobra.Command) string {
	flags := collectVisibleFlags(cmd)
	if len(flags) == 0 {
		return ""
	}

	title := "ALL FLAGS"
	if commandName(cmd) != "aim" {
		title = "FLAGS"
	}
	return renderNamedBlock(title, formatFlagLines(flags))
}

func renderManualExamplesSection(cmd *cobra.Command) string {
	lines := exampleLines(cmd)
	if len(lines) == 0 {
		return ""
	}
	return renderNamedBlock("EXAMPLES", lines)
}

func renderManualOptionsSection(cmd *cobra.Command) string {
	flags := collectVisibleFlags(cmd)
	if len(flags) == 0 {
		return ""
	}
	return renderNamedBlock("OPTIONS", formatFlagLines(flags))
}

func renderManualCommandsSection(cmd *cobra.Command) string {
	commands := orderedVisibleSubcommands(cmd)
	if len(commands) == 0 {
		return ""
	}
	return renderNamedBlock("COMMANDS", formatCommandLines(commands))
}

func renderSupportSection(cmd *cobra.Command) string {
	lines := []string{
		"Repository: " + rootCommandRepositoryURL,
		"Issues: " + rootCommandIssuesURL,
	}
	return renderNamedBlock("SUPPORT", lines)
}

func renderConciseMoreInfoSection(cmd *cobra.Command) string {
	if commandName(cmd) == "aim" {
		return `Use "aim --help" for the full reference or "aim help" for the manual.`
	}
	return fmt.Sprintf(`Use "%s --help" for the full reference or "aim help %s" for the manual.`, strings.TrimSpace(cmd.CommandPath()), commandName(cmd))
}

func renderFullMoreInfoSection(cmd *cobra.Command) string {
	lines := []string{
		"Docs: " + docsURLForCommand(cmd),
	}
	if commandName(cmd) == "aim" {
		lines = append(lines, `Manual: aim help`)
		lines = append(lines, "App updates: aim update")
		lines = append(lines, "CLI self-update: aim self-update")
		lines = append(lines, "Prompts: only on interactive stdin; pass --no-input to disable them.")
		lines = append(lines, "Cancellation: press Ctrl-C to cancel in-flight work.")
		lines = append(lines, "Settings: ${XDG_CONFIG_HOME:-~/.config}/aim/settings.toml (for example: network_timeout = \"30s\")")
		lines = append(lines, "Timeouts: metadata requests use network_timeout as a whole-request cap; downloads use it for connect/TLS/header waits.")
		lines = append(lines, "Retries: failed downloads and recent update checks can be reused on rerun.")
		lines = append(lines, "Writes: mutating commands take a state lock per AIM root.")
	} else {
		lines = append(lines, fmt.Sprintf("Manual: aim help %s", commandName(cmd)))
	}
	return renderNamedBlock("MORE INFO", lines)
}

func renderManualMoreInfoSection(cmd *cobra.Command) string {
	lines := []string{"Docs: " + docsURLForCommand(cmd)}
	if commandName(cmd) == "aim" {
		lines = append(lines, `Inline help: aim --help`)
		lines = append(lines, "App updates: aim update")
		lines = append(lines, "CLI self-update: aim self-update")
		lines = append(lines, "Prompts: only on interactive stdin; pass --no-input to disable them.")
		lines = append(lines, "Cancellation: press Ctrl-C to cancel in-flight work.")
		lines = append(lines, "Settings: ${XDG_CONFIG_HOME:-~/.config}/aim/settings.toml (for example: network_timeout = \"30s\")")
		lines = append(lines, "Timeouts: metadata requests use network_timeout as a whole-request cap; downloads use it for connect/TLS/header waits.")
		lines = append(lines, "Retries: failed downloads and recent update checks can be reused on rerun.")
		lines = append(lines, "Writes: mutating commands take a state lock per AIM root.")
	} else {
		lines = append(lines, fmt.Sprintf("Inline help: %s --help", strings.TrimSpace(cmd.CommandPath())))
	}
	return renderNamedBlock("MORE INFO", lines)
}

func renderNamedBlock(title string, lines []string) string {
	lines = filterEmptyLines(lines)
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(title) + "\n" + strings.Join(indentLines(lines), "\n")
}

func renderRootUpdateModesSection() string {
	return renderNamedBlock("UPDATE MODES", []string{
		"Use 'aim update' to check or apply AppImage updates.",
		"Use 'aim self-update' to update the aim CLI itself.",
	})
}

func usageLinesForCommand(cmd *cobra.Command) []string {
	if cmd == nil {
		return []string{"aim [flags] [command]"}
	}

	lines := []string{}
	base := strings.TrimSpace(commandPathWithUse(cmd))
	if base == "" {
		base = "aim"
	}

	switch {
	case commandName(cmd) == "aim":
		lines = append(lines, "aim [flags] [command]")
	case len(visibleSubcommands(cmd)) > 0:
		lines = append(lines, base+" [flags]")
		lines = append(lines, strings.TrimSpace(cmd.CommandPath())+" [command]")
	default:
		lines = append(lines, base+" [flags]")
	}

	return lines
}

func indentLines(lines []string) []string {
	indented := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, " ")
		if line == "" {
			indented = append(indented, "")
			continue
		}
		indented = append(indented, "  "+line)
	}
	return indented
}

func exampleLines(cmd *cobra.Command) []string {
	raw := strings.TrimSpace(cmd.Example)
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func visibleSubcommands(cmd *cobra.Command) []*cobra.Command {
	if cmd == nil {
		return nil
	}
	var commands []*cobra.Command
	for _, child := range cmd.Commands() {
		if child == nil || child.Hidden {
			continue
		}
		commands = append(commands, child)
	}
	return commands
}

func orderedVisibleSubcommands(cmd *cobra.Command) []*cobra.Command {
	commands := visibleSubcommands(cmd)
	sort.SliceStable(commands, func(i, j int) bool {
		return commandSortKey(commands[i]) < commandSortKey(commands[j])
	})
	return commands
}

func commonRootCommands(root *cobra.Command) []*cobra.Command {
	names := []string{"add", "update", "info", "list"}
	var commands []*cobra.Command
	for _, name := range names {
		if child := findVisibleSubcommand(root, name); child != nil {
			commands = append(commands, child)
		}
	}
	return commands
}

func commandSortKey(cmd *cobra.Command) string {
	if cmd == nil {
		return "zzzz"
	}
	name := cmd.Name()
	order := map[string]int{
		"add":    1,
		"update": 2,
		"info":   3,
		"list":   4,
		"remove": 5,
		"help":   6,
		"set":    1,
		"unset":  2,
	}
	if priority, ok := order[name]; ok {
		return fmt.Sprintf("%04d-%s", priority, name)
	}
	return fmt.Sprintf("9999-%s", name)
}

func formatCommandLines(commands []*cobra.Command) []string {
	if len(commands) == 0 {
		return nil
	}

	width := 0
	for _, cmd := range commands {
		if w := len(cmd.Name()); w > width {
			width = w
		}
	}

	lines := make([]string, 0, len(commands))
	for _, cmd := range commands {
		lines = append(lines, fmt.Sprintf("%-*s  %s", width, cmd.Name(), strings.TrimSpace(cmd.Short)))
	}
	return lines
}

func formatFlagLines(flags []*pflag.Flag) []string {
	if len(flags) == 0 {
		return nil
	}

	width := 0
	usages := make([]string, 0, len(flags))
	for _, flag := range flags {
		usage := formatInlineFlagUsage(flag)
		usages = append(usages, usage)
		if len(usage) > width {
			width = len(usage)
		}
	}

	lines := make([]string, 0, len(flags))
	for idx, flag := range flags {
		lines = append(lines, fmt.Sprintf("%-*s  %s", width, usages[idx], strings.TrimSpace(flag.Usage)))
	}
	return lines
}

func formatInlineFlagUsage(flag *pflag.Flag) string {
	if flag == nil {
		return ""
	}

	var parts []string
	if flag.Shorthand != "" && flag.ShorthandDeprecated == "" {
		shorthand := "-" + flag.Shorthand
		parts = append(parts, shorthand)
	}

	long := "--" + flag.Name
	if !isBooleanFlag(flag) {
		if metavar := flagMetavar(flag); metavar != "" {
			long += " " + metavar
		}
	}
	parts = append(parts, long)
	return strings.Join(parts, ", ")
}

func flagsForNames(cmd *cobra.Command, names []string) []*pflag.Flag {
	flags := make([]*pflag.Flag, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		flag := lookupFlag(cmd, name)
		if flag == nil || flag.Hidden || seen[flag.Name] {
			continue
		}
		seen[flag.Name] = true
		flags = append(flags, flag)
	}
	return flags
}

func commonFlagNamesForCommand(cmd *cobra.Command) []string {
	switch commandName(cmd) {
	case "aim":
		return []string{"help", "version", "json", "plain", "no-input"}
	case "add":
		return []string{"url", "github", "sha256", "no-input", "dry-run", "json"}
	case "info":
		return []string{"github", "no-input", "json"}
	case "list":
		return []string{"integrated", "unlinked", "json", "csv", "plain"}
	case "remove":
		return []string{"link", "no-input", "dry-run", "json"}
	case "update":
		return []string{"check-only", "set", "unset", "github", "zsync", "embedded", "yes", "dry-run", "json", "csv", "plain"}
	default:
		return []string{"dry-run", "json"}
	}
}

func docsURLForCommand(cmd *cobra.Command) string {
	switch commandName(cmd) {
	case "aim":
		return rootCommandDocsURL
	case "add":
		return "https://github.com/slobbe/appimage-manager#add"
	case "info":
		return "https://github.com/slobbe/appimage-manager#info"
	case "update":
		return "https://github.com/slobbe/appimage-manager#update"
	case "remove":
		return "https://github.com/slobbe/appimage-manager#remove"
	case "list":
		return "https://github.com/slobbe/appimage-manager#command-reference-list"
	default:
		return rootCommandDocsURL
	}
}

func filterEmptySections(sections []string) []string {
	out := make([]string, 0, len(sections))
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}
		out = append(out, section)
	}
	return out
}

func filterEmptyLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func findVisibleSubcommand(cmd *cobra.Command, name string) *cobra.Command {
	if cmd == nil {
		return nil
	}
	for _, child := range cmd.Commands() {
		if child.Hidden {
			continue
		}
		if child.Name() == name {
			return child
		}
	}
	return nil
}

func renderManPage(cmd *cobra.Command, sectionNum int) (string, error) {
	disableAutoGenTag(cmd)

	var buf bytes.Buffer
	header := &doc.GenManHeader{
		Title:   strings.ToUpper(manPageNameForCommand(cmd)),
		Section: strconv.Itoa(sectionNum),
		Source:  fmt.Sprintf("aim %s", cmd.Version),
		Manual:  rootCommandDescription,
	}
	if err := doc.GenMan(cmd, header, &buf); err != nil {
		return "", err
	}

	manPage := strings.TrimRight(buf.String(), "\n")
	manPage = stripManSection(manPage, "SEE ALSO")

	var extraSections []string
	if commands := renderManCommandsSection(cmd); commands != "" {
		extraSections = append(extraSections, commands)
	}
	if metadata := renderManMetadataSections(cmd); metadata != "" {
		extraSections = append(extraSections, metadata)
	}
	if len(extraSections) == 0 {
		return manPage + "\n", nil
	}

	return manPage + "\n" + strings.Join(extraSections, "\n") + "\n", nil
}

func renderManMetadataSections(cmd *cobra.Command) string {
	var sections []string

	if version := strings.TrimSpace(cmd.Version); version != "" {
		sections = append(sections, renderManSection("VERSION", version))
	}
	if author := strings.TrimSpace(rootCommandAuthor); author != "" {
		sections = append(sections, renderManSection("AUTHOR", author))
	}
	if copyright := strings.TrimSpace(rootCommandCopyright); copyright != "" {
		sections = append(sections, renderManSection("COPYRIGHT", copyright))
	}
	if license := strings.TrimSpace(rootCommandLicense); license != "" {
		sections = append(sections, renderManSection("LICENSE", license))
	}
	if repositoryURL := strings.TrimSpace(rootCommandRepositoryURL); repositoryURL != "" {
		sections = append(sections, renderManSection("REPOSITORY", repositoryURL))
	}
	if issuesURL := strings.TrimSpace(rootCommandIssuesURL); issuesURL != "" {
		sections = append(sections, renderManSection("ISSUES", issuesURL))
	}
	if docsURL := strings.TrimSpace(docsURLForCommand(cmd)); docsURL != "" {
		sections = append(sections, renderManSection("MORE INFO", docsURL))
	}
	if commandName(cmd) == "aim" {
		sections = append(sections, renderManSection("EXIT STATUS", formatExitStatusSection()))
		sections = append(sections, renderManSection("INTERACTIVITY", strings.Join([]string{
			"aim prompts only when stdin is an interactive terminal.",
			"Pass --no-input to disable all prompts and confirmations that require terminal input.",
			"Press Ctrl-C to cancel in-flight work.",
			"If aim ever accepts secret input in the future, it will not read it from plain flags.",
		}, "\n")))
		sections = append(sections, renderManSection("ROBUSTNESS", strings.Join([]string{
			"Network timeout is read from ${XDG_CONFIG_HOME:-~/.config}/aim/settings.toml.",
			"Example settings file: network_timeout = \"30s\"",
			"Metadata and update-check requests use network_timeout as a whole-request timeout; AppImage downloads use it for connection, TLS handshake, and response-header waits only.",
			"Failed long-running operations print a compact operation log after the main error.",
			"Interrupted downloads are staged under the AIM temp root and can be resumed on rerun when the server supports range requests.",
			"Recent successful update checks can be reused for up to 5 minutes on rerun.",
			"Mutating commands take a per-root state lock and fail fast if another aim process is already writing to the same root.",
		}, "\n")))
		sections = append(sections, renderManSection("ERROR REPORTING", strings.Join([]string{
			"Expected errors are rewritten to be actionable when possible.",
			"Unexpected internal failures suggest rerunning with --debug and reporting the issue at " + rootCommandIssuesURL + ".",
		}, "\n")))
	}

	return strings.Join(sections, "\n")
}

func renderManCommandsSection(root *cobra.Command) string {
	entries := collectVisibleCommands(root)
	if len(entries) == 0 {
		return ""
	}

	var sections []string
	sections = append(sections, ".SH COMMANDS")
	for _, cmd := range entries {
		sections = append(sections, renderManCommandEntry(cmd))
	}

	return strings.Join(sections, "\n")
}

func collectVisibleCommands(root *cobra.Command) []*cobra.Command {
	var commands []*cobra.Command
	for _, child := range root.Commands() {
		commands = appendVisibleCommands(commands, child)
	}
	return commands
}

func appendVisibleCommands(dst []*cobra.Command, cmd *cobra.Command) []*cobra.Command {
	if cmd == nil || cmd.Hidden {
		return dst
	}

	dst = append(dst, cmd)
	for _, child := range cmd.Commands() {
		dst = appendVisibleCommands(dst, child)
	}
	return dst
}

func renderManCommandEntry(cmd *cobra.Command) string {
	var sections []string

	sections = append(sections, fmt.Sprintf(".SS %s", escapeRoffText(renderManCommandLabel(cmd))))
	sections = append(sections, renderManParagraph("Synopsis", renderManSynopsis(cmd)))
	sections = append(sections, renderManParagraph("Description", renderManDescription(cmd)))

	if aliases := renderManAliases(cmd); aliases != "" {
		sections = append(sections, renderManParagraph("Aliases", aliases))
	}
	if examples := renderManExamples(cmd); examples != "" {
		sections = append(sections, renderManParagraph("Examples", examples))
	}
	if options := renderManFlagsSection(cmd); options != "" {
		sections = append(sections, options)
	}
	if subcommands := renderManSubcommandsSection(cmd); subcommands != "" {
		sections = append(sections, subcommands)
	}

	return strings.Join(sections, "\n")
}

func renderManCommandLabel(cmd *cobra.Command) string {
	return strings.TrimSpace(commandPathWithUse(cmd))
}

func renderManSynopsis(cmd *cobra.Command) string {
	return commandPathWithUse(cmd)
}

func commandPathWithUse(cmd *cobra.Command) string {
	use := strings.TrimSpace(cmd.Use)
	if use == "" {
		return strings.TrimSpace(cmd.CommandPath())
	}

	fields := strings.Fields(use)
	if len(fields) == 0 {
		return strings.TrimSpace(cmd.CommandPath())
	}

	base := strings.TrimSpace(cmd.CommandPath())
	if len(fields) == 1 {
		return base
	}

	return strings.TrimSpace(base + " " + strings.Join(fields[1:], " "))
}

func renderManDescription(cmd *cobra.Command) string {
	short := strings.TrimSpace(cmd.Short)
	long := strings.TrimSpace(cmd.Long)
	if long != "" && long != short {
		return long
	}
	return short
}

func renderManAliases(cmd *cobra.Command) string {
	aliases := visibleAliases(cmd)
	if len(aliases) == 0 {
		return ""
	}
	return strings.Join(aliases, ", ")
}

func visibleAliases(cmd *cobra.Command) []string {
	if cmd == nil || len(cmd.Aliases) == 0 {
		return nil
	}

	aliases := make([]string, 0, len(cmd.Aliases))
	for _, alias := range cmd.Aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		aliases = append(aliases, alias)
	}
	return aliases
}

func renderManExamples(cmd *cobra.Command) string {
	examples := strings.TrimSpace(cmd.Example)
	if examples == "" {
		return ""
	}

	lines := strings.Split(examples, "\n")
	for idx, line := range lines {
		lines[idx] = strings.TrimSpace(line)
	}
	return strings.Join(lines, "\n")
}

func renderManFlagsSection(cmd *cobra.Command) string {
	flags := collectVisibleFlags(cmd)
	if len(flags) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, ".TP")
	lines = append(lines, "\\fBOptions\\fP")
	for _, flag := range flags {
		lines = append(lines, ".TP")
		lines = append(lines, escapeRoffText(formatManFlagUsage(flag)))
		lines = append(lines, escapeRoffText(strings.TrimSpace(flag.Usage)))
	}

	return strings.Join(lines, "\n")
}

func collectVisibleFlags(cmd *cobra.Command) []*pflag.Flag {
	var flags []*pflag.Flag
	seen := map[string]bool{}

	appendFlags := func(flagSet *pflag.FlagSet) {
		if flagSet == nil {
			return
		}
		flagSet.VisitAll(func(flag *pflag.Flag) {
			if flag == nil || flag.Hidden || seen[flag.Name] {
				return
			}
			seen[flag.Name] = true
			flags = append(flags, flag)
		})
	}

	appendFlags(cmd.LocalFlags())
	appendFlags(cmd.InheritedFlags())
	return flags
}

func formatManFlagUsage(flag *pflag.Flag) string {
	if flag == nil {
		return ""
	}

	var parts []string
	if flag.Shorthand != "" && flag.ShorthandDeprecated == "" {
		shorthand := "-" + flag.Shorthand
		if !isBooleanFlag(flag) {
			if metavar := flagMetavar(flag); metavar != "" {
				shorthand += " " + metavar
			}
		}
		parts = append(parts, shorthand)
	}

	long := "--" + flag.Name
	if !isBooleanFlag(flag) {
		if metavar := flagMetavar(flag); metavar != "" {
			long += " " + metavar
		}
	}
	parts = append(parts, long)

	return strings.Join(parts, ", ")
}

func isBooleanFlag(flag *pflag.Flag) bool {
	if flag == nil || flag.Value == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(flag.Value.Type()), "bool")
}

func flagMetavar(flag *pflag.Flag) string {
	if flag == nil || flag.Value == nil {
		return ""
	}

	typeName := strings.TrimSpace(flag.Value.Type())
	if typeName == "" || strings.EqualFold(typeName, "bool") {
		return ""
	}
	return typeName
}

func renderManSubcommandsSection(cmd *cobra.Command) string {
	var entries []string
	for _, child := range cmd.Commands() {
		if child.Hidden {
			continue
		}
		entry := fmt.Sprintf("%s: %s", strings.TrimSpace(commandPathWithUse(child)), strings.TrimSpace(child.Short))
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return ""
	}

	return renderManParagraph("Subcommands", strings.Join(entries, "\n"))
}

func renderManParagraph(title, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}

	lines := []string{".TP", fmt.Sprintf("\\fB%s\\fP", escapeRoffText(title))}
	for _, line := range strings.Split(body, "\n") {
		lines = append(lines, escapeRoffText(strings.TrimSpace(line)))
	}
	return strings.Join(lines, "\n")
}

func renderManSection(title, body string) string {
	return fmt.Sprintf(".SH %s\n%s\n", escapeRoffText(title), escapeRoffText(body))
}

func stripManSection(manPage, section string) string {
	lines := strings.Split(manPage, "\n")
	header := ".SH " + section
	var out []string
	skipping := false

	for _, line := range lines {
		if strings.TrimSpace(line) == header {
			skipping = true
			continue
		}
		if skipping {
			if strings.HasPrefix(line, ".SH ") {
				skipping = false
			} else {
				continue
			}
		}
		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

func escapeRoffText(text string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "-", `\-`)
	return replacer.Replace(strings.TrimSpace(text))
}

func disableAutoGenTag(cmd *cobra.Command) {
	if cmd == nil {
		return
	}

	cmd.DisableAutoGenTag = true
	for _, child := range cmd.Commands() {
		disableAutoGenTag(child)
	}
}
