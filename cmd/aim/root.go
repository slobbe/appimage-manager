package main

import "github.com/spf13/cobra"

const (
	rootCommandDescription   = "Manage AppImages from the command line"
	rootCommandLong          = "Install, integrate, update, and remove AppImages on Linux."
	rootCommandAuthor        = "Sebastian Lobbe <slobbe@lobbe.cc>"
	rootCommandCopyright     = "Copyright (c) 2025 Sebastian Lobbe"
	rootCommandLicense       = "MIT"
	rootCommandRepositoryURL = "https://github.com/slobbe/appimage-manager"
	rootCommandIssuesURL     = "https://github.com/slobbe/appimage-manager/issues"
)

func newRootCommand(version string) *cobra.Command {
	root := &cobra.Command{
		Use:     "aim",
		Short:   rootCommandDescription,
		Long:    rootCommandLong,
		Version: version,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          RootCmd,
	}

	root.AddCommand(
		newAddCommand(),
		newIntegrateCommand(),
		newRemoveCommand(),
		newListCommand(),
		newShowCommand(),
		newInfoCommand(),
		newInspectCommand(),
		newInstallCommand(),
		newUpdateCommand(),
		newUpgradeCommand(),
	)
	root.InitDefaultVersionFlag()

	return root
}

func newAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <https-url|github:owner/repo|gitlab:namespace/project|id|path-to.AppImage>",
		Short: "Add a remote source, managed app, or local AppImage",
		RunE:  AddCmd,
	}
	flags := cmd.Flags()
	flags.String("asset", "", "asset filename pattern override for github:/gitlab: add sources")
	flags.String("sha256", "", "expected SHA-256 for direct https:// add sources")
	return cmd
}

func newIntegrateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "integrate <path-to.AppImage|id>",
		Short: "Integrate a local AppImage or reintegrate an existing ID",
		RunE:  IntegrateCmd,
	}
}

func newRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "remove <id>",
		Aliases: []string{"rm"},
		Short:   "Remove or unlink a managed AppImage",
		RunE:    RemoveCmd,
	}
	cmd.Flags().Bool("unlink", false, "remove only desktop integration; keep managed AppImage files")
	return cmd
}

func newListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List managed AppImages",
		RunE:    ListCmd,
	}
	flags := cmd.Flags()
	flags.BoolP("all", "a", false, "list all AppImages (default)")
	flags.BoolP("integrated", "i", false, "list integrated AppImages only")
	flags.BoolP("unlinked", "u", false, "list unlinked AppImages only")
	return cmd
}

func newShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <github:owner/repo|gitlab:namespace/project>",
		Short: "Show installability details for a package ref",
		RunE:  ShowCmd,
	}
}

func newInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "info <github:owner/repo|gitlab:namespace/project|id|path-to.AppImage>",
		Short: "Show package, managed app, or AppImage details",
		RunE:  InfoCmd,
	}
}

func newInspectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <id|path-to.AppImage>",
		Short: "Inspect a managed app or local AppImage",
		RunE:  InspectCmd,
	}
}

func newInstallCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "install <https-url|github:owner/repo|gitlab:namespace/project>",
		Aliases: []string{"i"},
		Short:   "Install from a remote source",
		RunE:    InstallCmd,
	}
	flags := cmd.Flags()
	flags.String("asset", "", "asset filename pattern override for github:/gitlab: install sources")
	flags.String("sha256", "", "expected SHA-256 for direct https:// install sources")
	return cmd
}

func newUpdateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "update [id]",
		Aliases: []string{"u"},
		Short:   "Check or apply updates, or set an update source",
		Long:    "Check or apply updates for managed apps, or manage configured update sources.",
		RunE:    UpdateCmd,
	}

	addUpdateSharedFlags(cmd)
	cmd.AddCommand(
		newUpdateSetCommand(),
		newUpdateUnsetCommand(),
		newUpdateCheckCommand(),
	)

	return cmd
}

func newUpdateSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "set <id>",
		Short: "Set the update source for a managed AppImage",
		RunE:  UpdateSetCmd,
	}
}

func newUpdateUnsetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "unset <id>",
		Short: "Unset the update source for a managed AppImage",
		RunE:  UpdateUnsetCmd,
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
	flags.BoolP("yes", "y", false, "apply updates without prompting")
	flags.BoolP("check-only", "c", false, "check only; do not apply updates")
	flags.String("github", "", "GitHub repo in the form owner/repo (for 'update set')")
	flags.String("asset", "", "asset filename pattern; defaults to \"*.AppImage\" for GitHub/GitLab (for 'update set')")
	flags.String("gitlab", "", "GitLab project path namespace/project (for 'update set')")
	flags.String("zsync-url", "", "direct zsync metadata URL (https, for 'update set')")
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

func newUpgradeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade aim to the latest stable release",
		Args:  cobra.NoArgs,
		RunE:  UpgradeCmd,
	}
}
