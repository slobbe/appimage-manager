package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/slobbe/appimage-manager/internal/app"
	"github.com/slobbe/appimage-manager/internal/cli"
	"github.com/slobbe/appimage-manager/internal/infra/appimage"
	"github.com/slobbe/appimage-manager/internal/infra/config"
	"github.com/slobbe/appimage-manager/internal/infra/desktop"
	"github.com/slobbe/appimage-manager/internal/infra/download"
	"github.com/slobbe/appimage-manager/internal/infra/fileutil"
	"github.com/slobbe/appimage-manager/internal/infra/github"
	"github.com/slobbe/appimage-manager/internal/infra/icon"
	"github.com/slobbe/appimage-manager/internal/infra/selfupdate"
	"github.com/slobbe/appimage-manager/internal/infra/storage"
	"github.com/slobbe/appimage-manager/internal/infra/xdg"
)

var version = "dev"

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
	githubHTTPClient := &http.Client{Timeout: 30 * time.Second}
	downloadHTTPClient := &http.Client{
		Timeout: 30 * time.Minute,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       90 * time.Second,
		},
	}

	service, err := app.NewService(app.ServiceDeps{
		Config:                      cfg,
		AppImages:                   appimage.Extractor{},
		DesktopEntries:              desktop.Discoverer{},
		Icons:                       icon.Discoverer{},
		AppImageInstaller:           appimage.NewInstaller(cfg.AppImageDir),
		IconInstaller:               icon.NewInstaller(cfg.IconDir),
		DesktopEntryInstaller:       desktop.NewInstaller(cfg.DesktopDir),
		ArtifactRemover:             fileutil.RemoveArtifact,
		ArtifactBackups:             fileutil.BackupManager{},
		ArtifactPaths:               fileutil.PathInspector{},
		DesktopIntegrationRefresher: desktop.NewRefresher(cfg.DesktopDir, cfg.IconDir),
		GitHubReleases:              github.NewClient(githubHTTPClient),
		Downloads:                   download.NewDownloader(downloadHTTPClient),
		SelfUpdater:                 selfupdate.Installer{},
		CurrentVersion:              version,
		Apps:                        storage.NewRepository(storagePath),
		Mutations:                   storage.NewMutationLocker(storagePath + ".operation.lock"),
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
