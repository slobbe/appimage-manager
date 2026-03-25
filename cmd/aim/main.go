//go:build !docgen

package main

import (
	"context"
	"log"
	"os"

	"github.com/slobbe/appimage-manager/internal/config"
)

func main() {
	if err := config.EnsureDirsExist(); err != nil {
		log.Fatal(err)
	}

	root := newRootCommand(version)
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)

	if handled, err := maybeRunRootUpgradeFlag(context.Background(), root, os.Args[1:]); handled {
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	root.SetArgs(os.Args[1:])
	root.SetVersionTemplate("{{.Version}}\n")

	if err := root.ExecuteContext(context.Background()); err != nil {
		log.Fatal(err)
	}
}
