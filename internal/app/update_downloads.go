package app

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
)

type preparedUpdateDownload struct {
	workspacePath string
	path          string
	cleanup       func()
	err           error
}

func cleanupUpdateDownloads(downloads []preparedUpdateDownload) {
	for _, download := range downloads {
		if download.cleanup != nil {
			download.cleanup()
		}
	}
}

// Each worker owns one result slot at a time. Workspaces remain alive until all
// downloads settle and the caller finishes the serial integration pass.
func (s *service) downloadGitHubUpdates(ctx context.Context, activity ActivityReporter, plans []githubUpdatePlan) ([]preparedUpdateDownload, error) {
	downloads := make([]preparedUpdateDownload, len(plans))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for worker := 0; worker < 3 && worker < len(plans); worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				downloads[i] = s.prepareGitHubUpdateDownload(ctx, activity, plans[i])
			}
		}()
	}

schedule:
	for i := range plans {
		if ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			break schedule
		case jobs <- i:
		}
	}
	close(jobs)
	workers.Wait()
	if err := ctx.Err(); err != nil {
		cleanupUpdateDownloads(downloads)
		return nil, err
	}
	return downloads, nil
}

func (s *service) prepareGitHubUpdateDownload(ctx context.Context, activity ActivityReporter, plan githubUpdatePlan) (prepared preparedUpdateDownload) {
	if s.downloads == nil {
		prepared.err = errors.New("asset downloader is required")
		return prepared
	}
	prepared.workspacePath, prepared.cleanup, prepared.err = createWorkspace(ctx)
	if prepared.err != nil {
		return prepared
	}

	prepared.path = filepath.Join(prepared.workspacePath, filepath.Base(plan.asset.Name))
	download := activity.Start(ctx, Activity{
		Kind:      ActivityKindDownloading,
		AppID:     plan.app.ID,
		Repo:      plan.app.UpdateSource.Repo,
		AssetName: plan.asset.Name,
		Total:     plan.asset.SizeBytes,
		Unit:      ActivityUnitBytes,
	})
	if err := ctx.Err(); err != nil {
		prepared.err = err
		download.Fail(err)
		return prepared
	}
	downloaded, err := s.downloads.Download(ctx, DownloadSource{
		URL:       plan.asset.DownloadURL,
		FileName:  plan.asset.Name,
		SizeBytes: plan.asset.SizeBytes,
	}, prepared.path, download)
	if err != nil {
		prepared.err = err
		download.Fail(err)
		return prepared
	}
	download.Done("Downloaded " + plan.asset.Name)
	if downloaded.Path != "" {
		prepared.path = downloaded.Path
	}
	return prepared
}
