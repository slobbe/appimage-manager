package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	bashCompletionRelativePath = "share/bash-completion/completions/aim"
	zshCompletionRelativePath  = "share/zsh/site-functions/_aim"
	fishCompletionRelativePath = "share/fish/vendor_completions.d/aim.fish"
)

func renderBashCompletion(cmd *cobra.Command) (string, error) {
	var buf bytes.Buffer
	if err := cmd.GenBashCompletionV2(&buf, true); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func renderZshCompletion(cmd *cobra.Command) (string, error) {
	var buf bytes.Buffer
	if err := cmd.GenZshCompletion(&buf); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func renderFishCompletion(cmd *cobra.Command) (string, error) {
	var buf bytes.Buffer
	if err := cmd.GenFishCompletion(&buf, true); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func writeCompletionFiles(cmd *cobra.Command, baseDir string) error {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return fmt.Errorf("completion output directory cannot be empty")
	}

	files := []struct {
		relativePath string
		render       func(*cobra.Command) (string, error)
	}{
		{relativePath: bashCompletionRelativePath, render: renderBashCompletion},
		{relativePath: zshCompletionRelativePath, render: renderZshCompletion},
		{relativePath: fishCompletionRelativePath, render: renderFishCompletion},
	}

	for _, file := range files {
		content, err := file.render(cmd)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(baseDir, file.relativePath)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
			return err
		}
	}

	return nil
}
