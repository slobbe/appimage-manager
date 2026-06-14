package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func NewCommand(root *cobra.Command) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "gen",
		Short:  "Generate developer assets",
		Long:   "Generate man pages and shell completions.",
		Hidden: true,
	}

	cmd.AddCommand(newManCommand(root))
	cmd.AddCommand(newCompletionCommand(root))

	return cmd
}

func newManCommand(root *cobra.Command) *cobra.Command {
	var dir string

	cmd := &cobra.Command{
		Use:   "man",
		Short: "Generate man pages",
		Long:  "Generate man pages from the Cobra command tree.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				return fmt.Errorf("--dir is required")
			}

			cleanDir := filepath.Clean(dir)
			if err := os.MkdirAll(cleanDir, 0o755); err != nil {
				return fmt.Errorf("create man directory %q: %w", cleanDir, err)
			}

			now := time.Now()
			header := &doc.GenManHeader{
				Title:   "AIM",
				Section: "1",
				Date:    &now,
				Source:  "aim",
				Manual:  "aim manual",
			}

			if err := doc.GenManTree(root, header, cleanDir); err != nil {
				return fmt.Errorf("generate man pages: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Generated man pages in %s\n", cleanDir)
			return nil
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "directory to write generated man pages")

	return cmd
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	var dir string

	cmd := &cobra.Command{
		Use:   "completion <shell>",
		Short: "Generate shell completion scripts",
		Long:  "Generate shell completion scripts from the Cobra command tree.",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return fmt.Errorf("requires exactly one shell: bash, zsh, fish, or powershell")
			}

			switch args[0] {
			case "bash", "zsh", "fish", "powershell":
				return nil
			default:
				return fmt.Errorf("unsupported shell %q: supported shells are bash, zsh, fish, and powershell", args[0])
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" {
				return fmt.Errorf("--dir is required")
			}

			cleanDir := filepath.Clean(dir)
			if err := os.MkdirAll(cleanDir, 0o755); err != nil {
				return fmt.Errorf("create completion directory %q: %w", cleanDir, err)
			}

			shell := args[0]
			filename, err := completionFilename(shell)
			if err != nil {
				return err
			}

			path := filepath.Join(cleanDir, filename)
			file, err := os.Create(path)
			if err != nil {
				return fmt.Errorf("create completion file %q: %w", path, err)
			}
			defer file.Close()

			switch shell {
			case "bash":
				err = root.GenBashCompletion(file)
			case "zsh":
				err = root.GenZshCompletion(file)
			case "fish":
				err = root.GenFishCompletion(file, true)
			case "powershell":
				err = root.GenPowerShellCompletion(file)
			}
			if err != nil {
				return fmt.Errorf("generate %s completion: %w", shell, err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Generated %s completion in %s\n", shell, path)
			return nil
		},
	}

	cmd.Flags().StringVar(&dir, "dir", "", "directory to write generated completion scripts")

	return cmd
}

func completionFilename(shell string) (string, error) {
	switch shell {
	case "bash":
		return "aim.bash", nil
	case "zsh":
		return "_aim", nil
	case "fish":
		return "aim.fish", nil
	case "powershell":
		return "aim.ps1", nil
	default:
		return "", fmt.Errorf("unsupported shell %q: supported shells are bash, zsh, fish, and powershell", shell)
	}
}
