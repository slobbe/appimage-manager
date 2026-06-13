package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli"
	"github.com/slobbe/appimage-manager/internal/infra/appimage"
	"github.com/slobbe/appimage-manager/internal/infra/config"
	"github.com/slobbe/appimage-manager/internal/infra/desktop"
	"github.com/slobbe/appimage-manager/internal/infra/download"
	"github.com/slobbe/appimage-manager/internal/infra/github"
	"github.com/slobbe/appimage-manager/internal/infra/icon"
	"github.com/slobbe/appimage-manager/internal/infra/migration"
	"github.com/slobbe/appimage-manager/internal/infra/selfupdate"
	"github.com/slobbe/appimage-manager/internal/infra/storage"
	"github.com/slobbe/appimage-manager/internal/infra/workspace"
	"github.com/slobbe/appimage-manager/internal/infra/xdg"
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
		Config:                      cfg,
		Workspaces:                  workspace.NewProvider(""),
		AppImages:                   appimage.NewExtractor(),
		AppImageStager:              appimage.NewStager(),
		DesktopEntries:              desktop.NewDiscoverer(),
		Icons:                       icon.NewDiscoverer(),
		AppImageInstaller:           appimage.NewInstaller(cfg.AppImageDir),
		AppImageRemover:             appimage.NewRemover(),
		IconInstaller:               icon.NewInstaller(cfg.IconDir),
		IconRemover:                 icon.NewRemover(),
		DesktopEntryInstaller:       desktop.NewInstaller(cfg.DesktopDir),
		DesktopEntryRemover:         desktop.NewRemover(),
		DesktopIntegrationRefresher: desktop.NewRefresher(cfg.DesktopDir, cfg.IconDir),
		GitHubReleases:              github.NewClient(),
		Downloads:                   download.NewDownloader(),
		SelfUpdater:                 selfupdate.NewInstaller(),
		CurrentVersion:              version,
		Apps:                        storage.NewRepository(storagePath),
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
