package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/slobbe/appimage-manager/internal/cli"
)

var version = "dev" // overridden by ldflags: `go build -ldflags "-X main.version=VERSION" -o ./bin/aim ./cmd/aim`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cli.ShouldGenerateDocsFromEnv() {
		if err := cli.GenerateDocs(version); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := cli.Run(ctx, version, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		os.Exit(cli.ExitCode(err))
	}
}
