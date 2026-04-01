package main

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/spf13/pflag"
)

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
			"Network timeout is read from ${XDG_CONFIG_HOME:-~/.config}/aim/settings.toml or <root>/config/aim/settings.toml when -C is used.",
			"Example settings file: network_timeout = \"30s\"",
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

	hidden := map[string]bool{}
	if commandName(cmd) == "migrate" {
		hidden["repair"] = true
	}

	aliases := make([]string, 0, len(cmd.Aliases))
	for _, alias := range cmd.Aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || hidden[alias] {
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
