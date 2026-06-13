package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"aim/internal/app"
	"aim/internal/cli"
	"aim/internal/infra/appimage"
	"aim/internal/infra/config"
	"aim/internal/infra/desktop"
	"aim/internal/infra/download"
	"aim/internal/infra/github"
	"aim/internal/infra/icon"
	"aim/internal/infra/migration"
	"aim/internal/infra/selfupdate"
	"aim/internal/infra/storage"
	"aim/internal/infra/workspace"
	"aim/internal/infra/xdg"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dirs, err := xdg.Resolve()
	if err != nil {
		exitWithError(err)
	}

	cfg, err := config.Load(xdg.ConfigFile(dirs), dirs)
	if err != nil {
		exitWithError(err)
	}

	storagePath := filepath.Join(xdg.DataDir(dirs), "apps.json")
	if _, err := migration.MigrateV1(ctx, migration.V1Options{
		SourcePath:  filepath.Join(xdg.StateDir(dirs), "apps.json"),
		DestPath:    storagePath,
		AppImageDir: cfg.AppImageDir,
		DesktopDir:  cfg.DesktopDir,
	}); err != nil {
		exitWithError(err)
	}

	service, err := app.NewService(app.ServiceDeps{
		Config:                cfg,
		Workspaces:            workspace.NewProvider(""),
		AppImages:             appimage.NewExtractor(),
		DesktopEntries:        desktop.NewDiscoverer(),
		Icons:                 icon.NewDiscoverer(),
		AppImageInstaller:     appimage.NewInstaller(cfg.AppImageDir),
		AppImageRemover:       appimage.NewRemover(),
		IconInstaller:         icon.NewInstaller(cfg.IconDir),
		IconRemover:           icon.NewRemover(),
		DesktopEntryInstaller: desktop.NewInstaller(cfg.DesktopDir),
		DesktopEntryRemover:   desktop.NewRemover(),
		GitHubReleases:        github.NewClient(),
		Downloads:             download.NewDownloader(),
		SelfUpdater:           selfupdate.NewInstaller(),
		CurrentVersion:        version,
		Apps:                  storage.NewRepository(storagePath),
	})
	if err != nil {
		exitWithError(err)
	}

	os.Exit(cli.Execute(
		ctx,
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		service,
		version,
	))

}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
