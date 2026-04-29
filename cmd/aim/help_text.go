package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

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
		lines = append(lines, "CLI self-update: aim --upgrade")
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
		lines = append(lines, "CLI self-update: aim --upgrade")
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
		"Use 'aim --upgrade' to upgrade the aim CLI itself.",
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
