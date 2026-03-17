package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/cpuguy83/go-md2man/v2/md2man"
	"github.com/urfave/cli/v3"
)

//go:embed aim_man.md.gotmpl
var manMarkdownTemplate string

type manTemplateData struct {
	Command       *cli.Command
	SectionNum    int
	Authors       []string
	GlobalOptions []string
	Commands      []string
}

func renderManPage(cmd *cli.Command, sectionNum int) (string, error) {
	markdown, err := renderManMarkdown(cmd, sectionNum)
	if err != nil {
		return "", err
	}

	return string(md2man.Render([]byte(markdown))), nil
}

func renderManMarkdown(cmd *cli.Command, sectionNum int) (string, error) {
	tmpl, err := template.New("aim-man").Parse(manMarkdownTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, manTemplateData{
		Command:       cmd,
		SectionNum:    sectionNum,
		Authors:       prepareManAuthors(cmd.Authors),
		GlobalOptions: prepareManFlags(cmd.VisibleFlags()),
		Commands:      prepareManCommands(cmd.VisibleCommands(), 0),
	}); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func prepareManAuthors(authors []any) []string {
	prepared := make([]string, 0, len(authors))
	for _, author := range authors {
		text := strings.TrimSpace(fmt.Sprint(author))
		if text == "" {
			continue
		}
		prepared = append(prepared, text)
	}

	return prepared
}

func prepareManFlags(flags []cli.Flag) []string {
	prepared := make([]string, 0, len(flags))
	for _, flag := range flags {
		text := strings.TrimSpace(flag.String())
		if text == "" {
			continue
		}

		parts := strings.SplitN(text, "\t", 2)
		label := strings.TrimSpace(parts[0])
		if len(parts) == 1 || strings.TrimSpace(parts[1]) == "" {
			prepared = append(prepared, fmt.Sprintf("**%s**", label))
			continue
		}

		prepared = append(prepared, fmt.Sprintf("**%s**: %s", label, strings.TrimSpace(parts[1])))
	}

	return prepared
}

func prepareManCommands(commands []*cli.Command, level int) []string {
	prepared := make([]string, 0, len(commands))
	for _, command := range commands {
		if command.Hidden {
			continue
		}

		prepared = append(prepared, prepareManCommand(command, level))
		if len(command.Commands) > 0 {
			prepared = append(prepared, prepareManCommands(command.Commands, level+1)...)
		}
	}

	return prepared
}

func prepareManCommand(command *cli.Command, level int) string {
	var buf strings.Builder
	heading := strings.Repeat("#", level+2)
	fmt.Fprintf(&buf, "%s %s\n\n", heading, strings.Join(command.Names(), ", "))

	if usage := strings.TrimSpace(command.Usage); usage != "" {
		buf.WriteString(usage)
		buf.WriteString("\n\n")
	}

	if usageText := strings.TrimSpace(command.UsageText); usageText != "" {
		buf.WriteString("```\n")
		buf.WriteString(usageText)
		if !strings.HasSuffix(usageText, "\n") {
			buf.WriteString("\n")
		}
		buf.WriteString("```\n\n")
	}

	for _, flag := range prepareManFlags(command.VisibleFlags()) {
		buf.WriteString(flag)
		buf.WriteString("\n\n")
	}

	return strings.TrimRight(buf.String(), "\n")
}
