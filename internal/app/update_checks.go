package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/slobbe/appimage-manager/internal/domain"
)

func (s *service) planGitHubUpdates(ctx context.Context, target string, activity ActivityReporter, ignoreReleaseAge bool) ([]githubUpdatePlan, []UpdateCandidate, []UpdateFailure, []UpdateSkip, []UpdateDeferred, int, error) {
	now := time.Now()
	task := activity.Start(ctx, Activity{Kind: ActivityKindCheckingUpdates})
	apps, err := s.updateScope(ctx, target)
	if err != nil {
		task.Fail(err)
		return nil, nil, nil, nil, nil, 0, err
	}

	type checkResult struct {
		plan     *githubUpdatePlan
		skip     *UpdateSkip
		deferred *UpdateDeferred
		err      error
	}
	results := make([]checkResult, len(apps))
	indices := make([]int, 0, len(apps))
	for i, installedApp := range apps {
		if err := ctx.Err(); err != nil {
			task.Fail(err)
			return nil, nil, nil, nil, nil, 0, err
		}
		if installedApp.UpdateSource.Kind != domain.UpdateSourceKindGitHub {
			skip := updateSkip(installedApp)
			results[i].skip = &skip
			continue
		}
		if strings.TrimSpace(installedApp.UpdateSource.Repo) == "" {
			results[i].skip = &UpdateSkip{
				AppID: installedApp.ID, Reason: UpdateSkipReasonInvalidSource,
				SourceKind: string(installedApp.UpdateSource.Kind),
			}
			continue
		}
		if s.githubReleases == nil {
			err := errors.New("github release finder is required")
			task.Fail(err)
			return nil, nil, nil, nil, nil, 0, err
		}
		indices = append(indices, i)
	}

	jobs := make(chan int)
	var workers sync.WaitGroup
	for worker := 0; worker < min(4, len(indices)); worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				installedApp := apps[i]
				release, err := s.githubReleaseForUpdateSource(ctx, installedApp.UpdateSource)
				if err != nil {
					results[i].err = err
					continue
				}
				asset, err := selectGitHubUpdateAsset(release, installedApp.UpdateSource)
				if err != nil {
					results[i].err = err
					continue
				}
				version, ok := updateVersion(release, asset)
				if ok && installedApp.HasUpdate(version) {
					deferUpdate, err := shouldDeferRelease(installedApp.UpdateSource, release, s.config.MinimumReleaseAgeHours, ignoreReleaseAge, now)
					if err != nil {
						results[i].err = err
						continue
					}
					if deferUpdate {
						results[i].deferred = &UpdateDeferred{
							AppID: installedApp.ID, CurrentVersion: installedApp.Version.String(), NewVersion: version.String(),
							PublishedAt: release.PublishedAt, MinimumReleaseAgeHours: s.config.MinimumReleaseAgeHours,
						}
						continue
					}
					results[i].plan = &githubUpdatePlan{app: installedApp, release: release, asset: asset, version: version}
				}
			}
		}()
	}

dispatch:
	for _, i := range indices {
		if ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
			break dispatch
		case jobs <- i:
		}
	}
	close(jobs)
	workers.Wait()
	plans := make([]githubUpdatePlan, 0)
	candidates := make([]UpdateCandidate, 0)
	failures := make([]UpdateFailure, 0)
	skipped := make([]UpdateSkip, 0)
	deferred := make([]UpdateDeferred, 0)
	// Each worker owns one result slot at a time; only this goroutine aggregates.
	for i, result := range results {
		switch {
		case result.deferred != nil:
			deferred = append(deferred, *result.deferred)
		case result.skip != nil:
			skipped = append(skipped, *result.skip)
		case result.err != nil:
			failures = append(failures, updateFailure(apps[i].ID, result.err))
		case result.plan != nil:
			plan := *result.plan
			plans = append(plans, plan)
			candidates = append(candidates, UpdateCandidate{
				ID: plan.app.ID, CurrentVersion: plan.app.Version.String(), NewVersion: plan.version.String(),
			})
		}
	}
	if err := ctx.Err(); err != nil {
		task.Fail(err)
		return nil, nil, nil, nil, deferred, 0, err
	}
	task.Done("Checked integrated apps")
	return plans, candidates, failures, skipped, deferred, len(apps), nil
}
