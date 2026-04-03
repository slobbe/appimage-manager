package main

import (
	"strings"

	"github.com/spf13/cobra"
)

const (
	rootCommandDescription   = "Manage AppImages from the terminal without manual desktop setup or update juggling"
	rootCommandLong          = "Install, integrate, update, and remove AppImages from one terminal tool."
	rootCommandAuthor        = "Sebastian Lobbe <slobbe@lobbe.cc>"
	rootCommandCopyright     = "Copyright (c) 2025 Sebastian Lobbe"
	rootCommandLicense       = "MIT"
	rootCommandRepositoryURL = "https://github.com/slobbe/appimage-manager"
	rootCommandIssuesURL     = "https://github.com/slobbe/appimage-manager/issues"
)

func newRootCommand(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "aim",
		Short: rootCommandDescription,
		Long:  rootCommandLong,
		Example: strings.TrimSpace(`
  aim --help
  aim --version
  aim --json
  aim add -n ./Example.AppImage
  aim update --yes
  aim --upgrade
`),
		Version: version,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return prepareRuntime(cmd)
		},
		RunE: RootCmd,
	}

	root.AddCommand(
		newAddCommand(),
		newRemoveCommand(),
		newListCommand(),
		newInfoCommand(),
		newUpdateCommand(),
	)
	root.SuggestionsMinimumDistance = 2
	root.Flags().Bool("upgrade", false, "upgrade aim via the official installer")
	persistentFlags := root.PersistentFlags()
	persistentFlags.BoolP("debug", "d", false, "enable diagnostic logging")
	persistentFlags.BoolP("quiet", "q", false, "suppress non-essential status output")
	persistentFlags.BoolP("dry-run", "n", false, "simulate changes without applying them")
	persistentFlags.BoolP("yes", "y", false, "skip confirmation prompts")
	persistentFlags.Bool("no-input", false, "disable interactive prompts")
	persistentFlags.Bool("json", false, "output formatted JSON")
	persistentFlags.Bool("csv", false, "output CSV where supported")
	persistentFlags.Bool("plain", false, "output plain tab-separated text")
	persistentFlags.Bool("no-color", false, "disable ANSI color output")
	root.InitDefaultVersionFlag()
	installHelpSystem(root)

	return root
}

func newAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [<id|Path/To.AppImage>]",
		Short: "Install an AppImage from a file, URL, or provider",
		Example: strings.TrimSpace(`
  aim add ./Example.AppImage
  aim add --url https://example.com/Example.AppImage --sha256 <sha256>
  aim add --github owner/repo
  aim add -n ./Example.AppImage --json
`),
		RunE: AddCmd,
	}
	flags := cmd.Flags()
	stringFlagWithMetavar(flags, "url", "", "", "direct https:// AppImage URL", "URL")
	stringFlagWithMetavar(flags, "github", "", "", "GitHub repo in the form owner/repo", "owner/repo")
	stringFlagWithMetavar(flags, "gitlab", "", "", "GitLab project path namespace/project", "namespace/project")
	flags.String("asset", "", "asset filename pattern override for GitHub/GitLab provider add sources")
	stringFlagWithMetavar(flags, "sha256", "", "", "expected SHA-256 for direct https:// add sources", "SHA256")
	return cmd
}

func newRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <id>",
		Aliases: []string{"rm"},
		Short:   "Remove a managed AppImage or only its desktop integration",
		Example: strings.TrimSpace(`
  aim remove example-app
  aim remove --link example-app
  aim remove -n example-app --json
`),
		RunE: RemoveCmd,
	}
	cmd.Flags().Bool("link", false, "remove only desktop integration; keep managed AppImage files")
	return cmd
}

func newListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List managed AppImages",
		Example: strings.TrimSpace(`
  aim list
  aim list --integrated
  aim list --json
  aim list --csv
`),
		RunE: ListCmd,
	}
	flags := cmd.Flags()
	flags.BoolP("all", "a", false, "list all AppImages (default)")
	flags.Bool("integrated", false, "list integrated AppImages only")
	flags.Bool("unlinked", false, "list unlinked AppImages only")
	return cmd
}

func newInfoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info [<id|Path/To.AppImage>]",
		Short: "Show AppImage or package details",
		Example: strings.TrimSpace(`
  aim info example-app
  aim info ./Example.AppImage
  aim info --github owner/repo
  aim info example-app --json
`),
		RunE: InfoCmd,
	}
	flags := cmd.Flags()
	stringFlagWithMetavar(flags, "github", "", "", "GitHub repo in the form owner/repo", "owner/repo")
	stringFlagWithMetavar(flags, "gitlab", "", "", "GitLab project path namespace/project", "namespace/project")
	return cmd
}

func newUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [id]",
		Aliases: []string{"u"},
		Short:   "Check, apply, or configure updates",
		Long:    "Check or apply updates for managed AppImages, or manage configured update sources.",
		Example: strings.TrimSpace(`
  aim update
  aim update --check-only --csv
  aim update --set example-app --github owner/repo
  aim update --unset example-app --yes
`),
		RunE: UpdateCmd,
	}

	addUpdateCheckFlags(cmd)
	addUpdateSourceFlags(cmd)

	return cmd
}

func addUpdateCheckFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.Bool("check-only", false, "check only; do not apply updates")
}

func addUpdateSourceFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	stringFlagWithMetavar(flags, "set", "", "", "set the update source for the given managed app id", "ID")
	stringFlagWithMetavar(flags, "unset", "", "", "unset the update source for the given managed app id", "ID")
	stringFlagWithMetavar(flags, "github", "", "", "GitHub repo in the form owner/repo (for 'aim update --set')", "owner/repo")
	flags.String("asset", "", "asset filename pattern; defaults to \"*.AppImage\" for GitHub/GitLab (for 'aim update --set')")
	stringFlagWithMetavar(flags, "gitlab", "", "", "GitLab project path namespace/project (for 'aim update --set')", "namespace/project")
	stringFlagWithMetavar(flags, "zsync", "", "", "direct zsync metadata URL (https, for 'aim update --set')", "URL")
	flags.Bool("embedded", false, "use the update source embedded in the current AppImage (for 'aim update --set')")
}

func mustMarkHidden(cmd *cobra.Command, name string) {
	if err := cmd.PersistentFlags().MarkHidden(name); err != nil {
		if err := cmd.Flags().MarkHidden(name); err != nil {
			panic(err)
		}
	}
}
