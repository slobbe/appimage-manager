package main

import "github.com/urfave/cli/v3"

func newRootCommand(version string) *cli.Command {
	return &cli.Command{
		Name:        "aim",
		Version:     version,
		Usage:       "Manage AppImages as desktop apps on Linux",
		Description: "aim manages AppImages on Linux. It installs, integrates, updates, and removes AppImages while keeping desktop integration and a local managed-app database in sync.",
		Authors: []any{
			"Sebastian Lobbe <slobbe@lobbe.cc>",
		},
		Copyright: "Copyright (c) 2025 Sebastian Lobbe",
		Action:    RootCmd,
		Commands: []*cli.Command{
			{
				Name:  "add",
				Usage: "Add a remote source, managed app, or local AppImage",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "target",
						UsageText: "<https-url|github:owner/repo|gitlab:namespace/project|id|path-to.AppImage>",
					},
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "asset",
						Usage: "asset filename pattern override for github:/gitlab: add sources",
					},
					&cli.StringFlag{
						Name:  "sha256",
						Usage: "expected SHA-256 for direct https:// add sources",
					},
				},
				Action: AddCmd,
			},
			{
				Name:  "integrate",
				Usage: "Integrate a local AppImage or reintegrate an existing ID",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "target",
						UsageText: "<path-to.AppImage|id>",
					},
				},
				Action: IntegrateCmd,
			},
			{
				Name:    "remove",
				Aliases: []string{"rm"},
				Usage:   "Remove or unlink a managed AppImage",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "id",
						UsageText: "<id>",
					},
				},
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "unlink",
						Value: false,
						Usage: "remove only desktop integration; keep managed AppImage files",
					},
				},
				Action: RemoveCmd,
			},
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "List managed AppImages",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "all",
						Aliases: []string{"a"},
						Value:   false,
						Usage:   "list all AppImages (default)",
					},
					&cli.BoolFlag{
						Name:    "integrated",
						Aliases: []string{"i"},
						Value:   false,
						Usage:   "list integrated AppImages only",
					},
					&cli.BoolFlag{
						Name:    "unlinked",
						Aliases: []string{"u"},
						Value:   false,
						Usage:   "list unlinked AppImages only",
					},
				},
				Action: ListCmd,
			},
			{
				Name:  "show",
				Usage: "Show installability details for a package ref",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "ref",
						UsageText: "<github:owner/repo|gitlab:namespace/project>",
					},
				},
				Action: ShowCmd,
			},
			{
				Name:  "info",
				Usage: "Show package, managed app, or AppImage details",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "target",
						UsageText: "<github:owner/repo|gitlab:namespace/project|id|path-to.AppImage>",
					},
				},
				Action: InfoCmd,
			},
			{
				Name:  "inspect",
				Usage: "Inspect a managed app or local AppImage",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "target",
						UsageText: "<id|path-to.AppImage>",
					},
				},
				Action: InspectCmd,
			},
			{
				Name:    "install",
				Aliases: []string{"i"},
				Usage:   "Install from a remote source",
				Arguments: []cli.Argument{
					&cli.StringArg{
						Name:      "ref",
						UsageText: "<https-url|github:owner/repo|gitlab:namespace/project>",
					},
				},
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "asset",
						Usage: "asset filename pattern override for github:/gitlab: install sources",
					},
					&cli.StringFlag{
						Name:  "sha256",
						Usage: "expected SHA-256 for direct https:// install sources",
					},
				},
				Action: InstallCmd,
			},
			{
				Name:      "update",
				Aliases:   []string{"u"},
				Usage:     "Check or apply updates, or set an update source",
				UsageText: "aim update [<id>] [--yes|-y] [--check-only|-c]\n   aim update set <id> (--github owner/repo [--asset \"*.AppImage\"] | --gitlab namespace/project [--asset \"*.AppImage\"] | --zsync-url <https-url> | --embedded)\n   aim update unset <id>",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "yes",
						Aliases: []string{"y"},
						Usage:   "apply updates without prompting",
					},
					&cli.BoolFlag{
						Name:    "check-only",
						Aliases: []string{"c"},
						Usage:   "check only; do not apply updates",
					},
					&cli.StringFlag{
						Name:  "github",
						Usage: "GitHub repo in the form owner/repo (for 'update set')",
					},
					&cli.StringFlag{
						Name:  "asset",
						Usage: "asset filename pattern; defaults to \"*.AppImage\" for GitHub/GitLab (for 'update set')",
					},
					&cli.StringFlag{
						Name:  "gitlab",
						Usage: "GitLab project path namespace/project (for 'update set')",
					},
					&cli.StringFlag{
						Name:  "zsync-url",
						Usage: "direct zsync metadata URL (https, for 'update set')",
					},
					&cli.BoolFlag{
						Name:  "embedded",
						Usage: "use the update source embedded in the current AppImage (for 'update set')",
					},
				},
				Action: UpdateCmd,
			},
			{
				Name:   "upgrade",
				Usage:  "Upgrade aim to the latest stable release",
				Action: UpgradeCmd,
			},
		},
	}
}
