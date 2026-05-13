package update

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	models "github.com/slobbe/appimage-manager/internal/domain"
	"github.com/slobbe/appimage-manager/internal/infra/download"
	fsys "github.com/slobbe/appimage-manager/internal/infra/filesystem"
	"github.com/slobbe/appimage-manager/internal/infra/zsync"
)

type ManagedUpdate struct {
	App            *models.App
	URL            string
	Asset          string
	Label          string
	Available      bool
	Latest         string
	ExpectedSHA1   string
	ExpectedSHA256 string
	Transport      string
	ZsyncURL       string
	FromKind       models.UpdateKind
}

type ManagedApplyStage string

const (
	ManagedApplyStageQueued    ManagedApplyStage = "queued"
	ManagedApplyStageZsync     ManagedApplyStage = "zsync"
	ManagedApplyStageDownload  ManagedApplyStage = "download"
	ManagedApplyStageVerify    ManagedApplyStage = "verify"
	ManagedApplyStageIntegrate ManagedApplyStage = "integrate"
	ManagedApplyStageDone      ManagedApplyStage = "done"
	ManagedApplyStageFailed    ManagedApplyStage = "failed"
)

type ManagedApplyEvent struct {
	AppID         string
	Index         int
	Total         int
	Stage         ManagedApplyStage
	Downloaded    int64
	DownloadTotal int64
	DownloadName  string
	Message       string
	Version       string
}

type ManagedApplyReporter interface {
	Event(ManagedApplyEvent)
}

type ManagedApplyReporterFunc func(ManagedApplyEvent)

func (f ManagedApplyReporterFunc) Event(event ManagedApplyEvent) {
	if f != nil {
		f(event)
	}
}

type IntegrateFunc func(context.Context, string, func(existing, incoming *models.UpdateSource) (bool, error)) (*models.App, error)

type Service struct {
	TempDir       string
	HTTPClient    *http.Client
	NowISO        func() string
	Zsync         zsync.Runner
	Integrate     IntegrateFunc
	DownloadAsset func(context.Context, string, string, func(downloaded, total int64)) error
}

func (s Service) ApplyManagedUpdate(ctx context.Context, update ManagedUpdate, reporter ManagedApplyReporter) (*models.App, error) {
	emitManagedApplyEvent(reporter, ManagedApplyEvent{Stage: ManagedApplyStageQueued})

	if strings.TrimSpace(update.URL) == "" {
		err := fmt.Errorf("missing download URL")
		emitManagedApplyEvent(reporter, ManagedApplyEvent{Stage: ManagedApplyStageFailed, Message: err.Error()})
		return nil, err
	}

	appID := managedUpdateAppID(update)
	fileName := ManagedUpdateDownloadFilename(update.Asset, update.URL)
	downloadPath, err := s.stableManagedUpdateDownloadDestination(update.URL, appID+"-"+fileName)
	if err != nil {
		emitManagedApplyEvent(reporter, ManagedApplyEvent{Stage: ManagedApplyStageFailed, Message: err.Error()})
		return nil, err
	}

	usedZsync := false
	if strings.TrimSpace(update.ZsyncURL) != "" {
		emitManagedApplyEvent(reporter, ManagedApplyEvent{Stage: ManagedApplyStageZsync})
		if err := s.ApplyManagedZsyncUpdate(ctx, update, downloadPath); err == nil {
			usedZsync = true
		}
	}

	if !usedZsync {
		emitManagedApplyEvent(reporter, ManagedApplyEvent{
			Stage:        ManagedApplyStageDownload,
			DownloadName: fileName,
		})
		if err := s.downloadManagedUpdateAsset(ctx, update.URL, downloadPath, func(downloaded, total int64) {
			emitManagedApplyEvent(reporter, ManagedApplyEvent{
				Stage:         ManagedApplyStageDownload,
				Downloaded:    downloaded,
				DownloadTotal: total,
				DownloadName:  fileName,
			})
		}); err != nil {
			emitManagedApplyEvent(reporter, ManagedApplyEvent{Stage: ManagedApplyStageFailed, Message: err.Error()})
			return nil, err
		}
	}

	emitManagedApplyEvent(reporter, ManagedApplyEvent{Stage: ManagedApplyStageVerify})
	if err := VerifyDownloadedUpdate(downloadPath, update); err != nil {
		emitManagedApplyEvent(reporter, ManagedApplyEvent{Stage: ManagedApplyStageFailed, Message: err.Error()})
		return nil, err
	}

	emitManagedApplyEvent(reporter, ManagedApplyEvent{Stage: ManagedApplyStageIntegrate})
	app, err := s.integrate(ctx, downloadPath)
	if err != nil {
		emitManagedApplyEvent(reporter, ManagedApplyEvent{Stage: ManagedApplyStageFailed, Message: err.Error()})
		return nil, err
	}

	if update.App != nil {
		app.Source = update.App.Source
		app.Update = update.App.Update
		if strings.TrimSpace(update.App.AddedAt) != "" {
			app.AddedAt = update.App.AddedAt
		}
		app.LastCheckedAt = update.App.LastCheckedAt
		if strings.TrimSpace(update.App.ID) != "" && strings.TrimSpace(update.App.ID) != strings.TrimSpace(app.ID) {
			app.ReplacesID = update.App.ID
		}
	}

	emitManagedApplyEvent(reporter, ManagedApplyEvent{
		Stage:   ManagedApplyStageDone,
		Version: app.Version,
	})
	RemoveManagedUpdateDownload(downloadPath)
	return app, nil
}

func (s Service) ApplyManagedZsyncUpdate(ctx context.Context, update ManagedUpdate, destination string) error {
	if update.App == nil {
		return fmt.Errorf("missing app")
	}
	return s.Zsync.Apply(ctx, update.App.ExecPath, update.ZsyncURL, destination)
}

func (s Service) DownloadManagedUpdateAsset(ctx context.Context, assetURL, destination string, onProgress func(downloaded, total int64)) error {
	return s.downloadManagedUpdateAsset(ctx, assetURL, destination, onProgress)
}

func (s Service) downloadManagedUpdateAsset(ctx context.Context, assetURL, destination string, onProgress func(downloaded, total int64)) error {
	if s.DownloadAsset != nil {
		return s.DownloadAsset(ctx, assetURL, destination, onProgress)
	}
	return (download.StagedDownloader{
		Client: s.HTTPClient,
		NowISO: s.NowISO,
	}).Download(ctx, assetURL, destination, func(event download.Progress) {
		if onProgress != nil {
			onProgress(event.Downloaded, event.Total)
		}
	})
}

func (s Service) stableManagedUpdateDownloadDestination(assetURL, nameHint string) (string, error) {
	return download.StableDestination(filepath.Join(s.TempDir, "downloads"), assetURL, nameHint)
}

func (s Service) integrate(ctx context.Context, downloadPath string) (*models.App, error) {
	if s.Integrate == nil {
		return nil, fmt.Errorf("managed update integration is not configured")
	}
	return s.Integrate(ctx, downloadPath, func(existing, incoming *models.UpdateSource) (bool, error) {
		return false, nil
	})
}

func VerifyDownloadedUpdate(downloadPath string, update ManagedUpdate) error {
	return fsys.VerifyHashes(downloadPath, update.ExpectedSHA256, update.ExpectedSHA1)
}

func ManagedUpdateDownloadFilename(assetName, downloadURL string) string {
	return download.AppImageFilename(assetName, downloadURL)
}

func RemoveManagedUpdateDownload(downloadPath string) {
	download.RemoveStaged(downloadPath)
}

func managedUpdateAppID(update ManagedUpdate) string {
	if update.App != nil && strings.TrimSpace(update.App.ID) != "" {
		return strings.TrimSpace(update.App.ID)
	}
	return "app"
}

func emitManagedApplyEvent(reporter ManagedApplyReporter, event ManagedApplyEvent) {
	if reporter != nil {
		reporter.Event(event)
	}
}
