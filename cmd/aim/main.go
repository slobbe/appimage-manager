//go:build !docgen

package main

import (
	"context"
	"log"
	"os"

	"github.com/slobbe/appimage-manager/internal/config"
	repo "github.com/slobbe/appimage-manager/internal/repository"
)

func main() {
	if err := config.EnsureDirsExist(); err != nil {
		log.Fatal(err)
	}

	if err := repo.MigrateToCurrentPaths(); err != nil {
		log.Fatal(err)
	}

	root := newRootCommand(version)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.SetArgs(os.Args[1:])
	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")

	if err := root.ExecuteContext(context.Background()); err != nil {
		log.Fatal(err)
	}
}
