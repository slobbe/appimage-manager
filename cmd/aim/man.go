package main

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func renderManPage(cmd *cobra.Command, sectionNum int) (string, error) {
	disableAutoGenTag(cmd)

	var buf bytes.Buffer
	header := &doc.GenManHeader{
		Title:   strings.ToUpper(cmd.Name()),
		Section: strconv.Itoa(sectionNum),
		Source:  fmt.Sprintf("%s %s", cmd.Name(), cmd.Version),
		Manual:  rootCommandDescription,
	}
	if err := doc.GenMan(cmd, header, &buf); err != nil {
		return "", err
	}

	manPage := strings.TrimRight(buf.String(), "\n")
	extra := renderManMetadataSections(cmd)
	if extra == "" {
		return manPage + "\n", nil
	}

	return manPage + "\n" + extra, nil
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

	return strings.Join(sections, "\n")
}

func renderManSection(title, body string) string {
	return fmt.Sprintf(".SH %s\n%s\n", escapeRoffText(title), escapeRoffText(body))
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
