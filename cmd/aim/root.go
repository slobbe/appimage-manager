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
		newMigrateCommand(),
		newUpdateCommand(),
	)
	root.SuggestionsMinimumDistance = 2
	root.Flags().Bool("upgrade", false, "upgrade aim via the official installer")
	persistentFlags := root.PersistentFlags()
	persistentFlags.BoolP("debug", "d", false, "enable diagnostic logging")
	persistentFlags.Bool("verbose", false, "enable diagnostic logging (deprecated: use --debug)")
	persistentFlags.BoolP("quiet", "q", false, "suppress non-essential status output")
	persistentFlags.BoolP("dry-run", "n", false, "simulate changes without applying them")
	persistentFlags.BoolP("yes", "y", false, "skip confirmation prompts")
	persistentFlags.Bool("no-input", false, "disable interactive prompts")
	persistentFlags.Bool("json", false, "output formatted JSON")
	persistentFlags.Bool("csv", false, "output CSV where supported")
	persistentFlags.Bool("plain", false, "output plain tab-separated text")
	persistentFlags.Bool("no-color", false, "disable ANSI color output")
	mustMarkHidden(root, "verbose")
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
		Short:   "Remove or unlink a managed AppImage",
		Example: strings.TrimSpace(`
  aim remove example-app
  aim remove --unlink example-app
  aim remove -n example-app --json
`),
		RunE: RemoveCmd,
	}
	cmd.Flags().Bool("unlink", false, "remove only desktop integration; keep managed AppImage files")
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
	flags.BoolP("integrated", "i", false, "list integrated AppImages only")
	flags.BoolP("unlinked", "u", false, "list unlinked AppImages only")
	mustMarkShorthandDeprecated(cmd, "integrated", "use --integrated")
	mustMarkShorthandDeprecated(cmd, "unlinked", "use --unlinked")
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
	cmd.AddCommand(
		newUpdateCheckCommand(),
		newRemovedUpdateSetCommand(),
		newRemovedUpdateUnsetCommand(),
	)

	return cmd
}

func newMigrateCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "migrate [id]",
		Aliases: []string{"repair"},
		Short:   "Migrate managed AppImage state and repair desktop integration",
		Long:    "Migrate managed AppImage state, repair legacy paths, and reconcile desktop integration. This command may inspect AppImages and can take longer than ordinary commands.",
		Example: strings.TrimSpace(`
  aim migrate
  aim migrate example-app
  aim migrate -n --json
`),
		Args: cobra.MaximumNArgs(1),
		RunE: MigrateCmd,
	}
}

func newUpdateCheckCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "check",
		Short:  "Removed update check command stub",
		Hidden: true,
		RunE:   UpdateCheckRemovedCmd,
	}
}

func newRemovedUpdateSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:                "set",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return removedUpdateSetCommandError()
		},
	}
}

func newRemovedUpdateUnsetCommand() *cobra.Command {
	return &cobra.Command{
		Use:                "unset",
		Hidden:             true,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = args
			return removedUpdateUnsetCommandError()
		},
	}
}

func addUpdateCheckFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.BoolP("check-only", "c", false, "check only; do not apply updates")
	mustMarkShorthandDeprecated(cmd, "check-only", "use --check-only")
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
	flags.String("manifest-url", "", "deprecated manifest update source flag")
	flags.String("url", "", "deprecated direct update source flag")
	flags.String("sha256", "", "deprecated update source checksum flag")

	mustMarkHidden(cmd, "manifest-url")
	mustMarkHidden(cmd, "url")
	mustMarkHidden(cmd, "sha256")
}

func mustMarkHidden(cmd *cobra.Command, name string) {
	if err := cmd.PersistentFlags().MarkHidden(name); err != nil {
		if err := cmd.Flags().MarkHidden(name); err != nil {
			panic(err)
		}
	}
}

func mustMarkShorthandDeprecated(cmd *cobra.Command, name, message string) {
	if err := cmd.Flags().MarkShorthandDeprecated(name, message); err != nil {
		panic(err)
	}
}
