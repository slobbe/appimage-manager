//go:build !docgen

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/slobbe/appimage-manager/internal/config"
	repo "github.com/slobbe/appimage-manager/internal/repository"
)

func main() {
	cli.VersionPrinter = func(cmd *cli.Command) {
		root := cmd.Root()
		_, _ = fmt.Fprintf(root.Writer, "%s %s\n", root.Name, root.Version)
	}

	if err := config.EnsureDirsExist(); err != nil {
		log.Fatal(err)
	}

	if err := repo.MigrateToCurrentPaths(); err != nil {
		log.Fatal(err)
	}

	if err := newRootCommand(version).Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}
