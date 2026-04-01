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
  aim --output json
  aim -C /tmp/aim-state list --output json
  aim add --dry-run ./Example.AppImage
  aim update --yes
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
	root.Flags().BoolP("upgrade", "U", false, "upgrade aim via the official installer")
	persistentFlags := root.PersistentFlags()
	persistentFlags.Bool("verbose", false, "enable verbose diagnostic logging")
	persistentFlags.BoolP("quiet", "q", false, "suppress non-essential status output")
	stringFlagWithMetavar(persistentFlags, "config", "C", "", "use an alternate AIM state root", "DIR")
	persistentFlags.Bool("dry-run", false, "simulate changes without applying them")
	persistentFlags.BoolP("yes", "y", false, "skip confirmation prompts")
	persistentFlags.String("output", outputText, "output format: text, json, or csv")
	root.InitDefaultVersionFlag()
	installHelpSystem(root)

	return root
}

func newAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [<https-url|github-url|gitlab-url|id|Path/To.AppImage>]",
		Short: "Install an AppImage from a file, URL, or provider",
		Example: strings.TrimSpace(`
  aim add ./Example.AppImage
  aim add https://example.com/Example.AppImage --sha256 <sha256>
  aim add --github owner/repo
  aim add --dry-run ./Example.AppImage --output json
`),
		RunE: AddCmd,
	}
	flags := cmd.Flags()
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
  aim remove --dry-run example-app --output json
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
  aim list --output json
  aim list --output csv
`),
		RunE: ListCmd,
	}
	flags := cmd.Flags()
	flags.BoolP("all", "a", false, "list all AppImages (default)")
	flags.BoolP("integrated", "i", false, "list integrated AppImages only")
	flags.BoolP("unlinked", "u", false, "list unlinked AppImages only")
	return cmd
}

func newInfoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info [<github-url|gitlab-url|id|Path/To.AppImage>]",
		Short: "Show AppImage or package details",
		Example: strings.TrimSpace(`
  aim info example-app
  aim info ./Example.AppImage
  aim info --github owner/repo
  aim info example-app --output json
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
  aim update --check-only --output csv
  aim update --yes
  aim update example-app --dry-run --output json
`),
		RunE: UpdateCmd,
	}

	addUpdateSharedFlags(cmd)
	cmd.AddCommand(
		newUpdateSetCommand(),
		newUpdateUnsetCommand(),
		newUpdateCheckCommand(),
	)

	return cmd
}

func newMigrateCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "migrate [id]",
		Aliases: []string{"repair"},
		Short:   "Run migration and desktop integration repair",
		Long:    "Repair managed AppImage state, migrate legacy paths, and reconcile desktop integration. This command may inspect AppImages and can take longer than ordinary commands.",
		Example: strings.TrimSpace(`
  aim migrate
  aim migrate example-app
  aim migrate --dry-run --output json
`),
		Args: cobra.MaximumNArgs(1),
		RunE: MigrateCmd,
	}
}

func newUpdateSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set <id>",
		Short: "Set the update source for a managed AppImage",
		Example: strings.TrimSpace(`
  aim update set example-app --github owner/repo
  aim update set example-app --embedded
  aim update set example-app --zsync https://example.com/Example.AppImage.zsync --dry-run --output json
`),
		RunE: UpdateSetCmd,
	}
}

func newUpdateUnsetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "unset <id>",
		Short: "Unset the update source for a managed AppImage",
		Example: strings.TrimSpace(`
  aim update unset example-app
  aim update unset example-app --dry-run --output json
`),
		RunE: UpdateUnsetCmd,
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

func addUpdateSharedFlags(cmd *cobra.Command) {
	flags := cmd.PersistentFlags()
	flags.BoolP("check-only", "c", false, "check only; do not apply updates")
	stringFlagWithMetavar(flags, "github", "", "", "GitHub repo in the form owner/repo (for 'update set')", "owner/repo")
	flags.String("asset", "", "asset filename pattern; defaults to \"*.AppImage\" for GitHub/GitLab (for 'update set')")
	stringFlagWithMetavar(flags, "gitlab", "", "", "GitLab project path namespace/project (for 'update set')", "namespace/project")
	stringFlagWithMetavar(flags, "zsync", "", "", "direct zsync metadata URL (https, for 'update set')", "URL")
	flags.Bool("embedded", false, "use the update source embedded in the current AppImage (for 'update set')")
	flags.String("manifest-url", "", "deprecated manifest update source flag")
	flags.String("url", "", "deprecated direct update source flag")
	flags.String("sha256", "", "deprecated update source checksum flag")

	mustMarkHidden(cmd, "manifest-url")
	mustMarkHidden(cmd, "url")
	mustMarkHidden(cmd, "sha256")
}

func mustMarkHidden(cmd *cobra.Command, name string) {
	if err := cmd.PersistentFlags().MarkHidden(name); err != nil {
		panic(err)
	}
}
