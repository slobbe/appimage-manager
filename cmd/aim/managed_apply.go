package main

import (
	"context"
	"fmt"
	"strings"
	"sync"

	models "github.com/slobbe/appimage-manager/internal/types"
	"github.com/spf13/cobra"
)

const maxManagedApplyWorkers = 5

var managedApplyRenderInterval = progressThrottleInterval

type managedApplyStage string

const (
	managedApplyStageQueued    managedApplyStage = "queued"
	managedApplyStageZsync     managedApplyStage = "zsync"
	managedApplyStageDownload  managedApplyStage = "download"
	managedApplyStageVerify    managedApplyStage = "verify"
	managedApplyStageIntegrate managedApplyStage = "integrate"
	managedApplyStageDone      managedApplyStage = "done"
	managedApplyStageFailed    managedApplyStage = "failed"
)

type managedApplyEvent struct {
	AppID         string
	Index         int
	Total         int
	Stage         managedApplyStage
	Downloaded    int64
	DownloadTotal int64
	Message       string
	Version       string
}

type managedApplyReporter interface {
	Event(managedApplyEvent)
}

type managedApplyReporterFunc func(managedApplyEvent)

func (f managedApplyReporterFunc) Event(event managedApplyEvent) {
	if f != nil {
		f(event)
	}
}

type managedApplyResult struct {
	index      int
	app        *models.App
	updatedApp *models.App
	err        error
}

type managedApplyController interface {
	Event(managedApplyEvent)
	Finish([]managedApplyResult)
}

type batchManagedApplyController struct {
	cmd       *cobra.Command
	total     int
	progress  progressHandle
	mu        sync.Mutex
	completed []bool
	failures  int
}

type singleManagedApplyController struct {
	cmd           *cobra.Command
	appID         string
	handle        progressHandle
	handleMode    progressMode
	downloaded    int64
	downloadTotal int64
}

func runManagedApplies(ctx context.Context, cmd *cobra.Command, pending []pendingManagedUpdate) []managedApplyResult {
	if len(pending) == 0 {
		return nil
	}

	controller := newManagedApplyController(cmd, pending)
	results := make([]managedApplyResult, len(pending))
	jobs := make(chan int, len(pending))
	workerCount := managedApplyWorkerCount(len(pending))

	var wg sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for idx := range jobs {
				item := pending[idx]
				reporter := managedApplyReporterFunc(func(event managedApplyEvent) {
					if item.App != nil && strings.TrimSpace(event.AppID) == "" {
						event.AppID = item.App.ID
					}
					event.Index = idx
					event.Total = len(pending)
					controller.Event(event)
				})

				updatedApp, err := runManagedApply(ctx, item, reporter)
				results[idx] = managedApplyResult{
					index:      idx,
					app:        item.App,
					updatedApp: updatedApp,
					err:        err,
				}
			}
		}()
	}

	for idx := range pending {
		jobs <- idx
	}
	close(jobs)
	wg.Wait()

	controller.Finish(results)
	return results
}

func managedApplyWorkerCount(total int) int {
	if total <= 0 {
		return 0
	}
	if total < maxManagedApplyWorkers {
		return total
	}
	return maxManagedApplyWorkers
}

func emitManagedApplyEvent(reporter managedApplyReporter, event managedApplyEvent) {
	if reporter != nil {
		reporter.Event(event)
	}
}

func newManagedApplyController(cmd *cobra.Command, pending []pendingManagedUpdate) managedApplyController {
	if len(pending) == 1 {
		appID := ""
		if pending[0].App != nil {
			appID = strings.TrimSpace(pending[0].App.ID)
		}
		return newSingleManagedApplyController(cmd, appID)
	}
	return newBatchManagedApplyController(cmd, len(pending))
}

func newBatchManagedApplyController(cmd *cobra.Command, total int) *batchManagedApplyController {
	return &batchManagedApplyController{
		cmd:       cmd,
		total:     total,
		progress:  newCountProgress(cmd, managedApplyAggregateDescription(total, 0, 0), int64(total)),
		completed: make([]bool, total),
	}
}

func (c *batchManagedApplyController) Event(event managedApplyEvent) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if event.Index < 0 || event.Index >= len(c.completed) {
		return
	}

	if event.Stage != managedApplyStageDone && event.Stage != managedApplyStageFailed {
		return
	}
	if c.completed[event.Index] {
		return
	}

	c.completed[event.Index] = true
	if event.Stage == managedApplyStageFailed {
		c.failures++
	}
	if c.progress != nil {
		c.progress.Add(1)
		c.progress.Describe(managedApplyAggregateDescription(c.total, completedCount(c.completed), c.failures))
	}
}

func (c *batchManagedApplyController) Finish(results []managedApplyResult) {
	if c == nil {
		return
	}
	if c.progress != nil {
		c.progress.Finish()
	}

	failures := 0
	successes := 0
	for _, result := range results {
		if result.err != nil {
			failures++
			appID := "<unknown>"
			if result.app != nil && strings.TrimSpace(result.app.ID) != "" {
				appID = result.app.ID
			}
			printError(c.cmd, fmt.Sprintf("Failed: %s: %v", appID, result.err))
			continue
		}
		if result.updatedApp != nil {
			successes++
		}
	}

	summary := fmt.Sprintf("Updated %d app(s); %d failed", successes, failures)
	if failures > 0 {
		printWarning(c.cmd, summary)
		return
	}
	printSuccess(c.cmd, summary)
}

func newSingleManagedApplyController(cmd *cobra.Command, appID string) *singleManagedApplyController {
	if strings.TrimSpace(appID) == "" {
		appID = "app"
	}
	return &singleManagedApplyController{
		cmd:        cmd,
		appID:      appID,
		handle:     newSpinnerProgress(cmd, fmt.Sprintf("Updating %s", appID)),
		handleMode: progressModeSpinner,
	}
}

func (c *singleManagedApplyController) Event(event managedApplyEvent) {
	if c == nil {
		return
	}
	if strings.TrimSpace(event.AppID) != "" {
		c.appID = strings.TrimSpace(event.AppID)
	}

	switch event.Stage {
	case managedApplyStageDownload:
		c.updateDownloadProgress(event.Downloaded, event.DownloadTotal)
	case managedApplyStageZsync:
		c.setSpinnerDescription(fmt.Sprintf("Applying delta update to %s", c.appID))
	case managedApplyStageVerify:
		c.setSpinnerDescription(fmt.Sprintf("Verifying %s", c.appID))
	case managedApplyStageIntegrate:
		c.setSpinnerDescription(fmt.Sprintf("Integrating %s", c.appID))
	case managedApplyStageQueued:
		c.setSpinnerDescription(fmt.Sprintf("Preparing %s", c.appID))
	case managedApplyStageDone, managedApplyStageFailed:
		// Final state is rendered after the worker result is collected.
	}
}

func (c *singleManagedApplyController) Finish(results []managedApplyResult) {
	if c == nil {
		return
	}
	if c.handle != nil {
		c.handle.Clear()
		c.handle = nil
	}

	failures := 0
	successes := 0
	for _, result := range results {
		if result.err != nil {
			failures++
			appID := c.appID
			if result.app != nil && strings.TrimSpace(result.app.ID) != "" {
				appID = result.app.ID
			}
			printError(c.cmd, fmt.Sprintf("Failed: %s: %v", appID, result.err))
			continue
		}
		if result.updatedApp != nil {
			successes++
		}
	}

	summary := fmt.Sprintf("Updated %d app(s); %d failed", successes, failures)
	if failures > 0 {
		printWarning(c.cmd, summary)
		return
	}
	printSuccess(c.cmd, summary)
}

func (c *singleManagedApplyController) setSpinnerDescription(description string) {
	description = strings.TrimSpace(description)
	if description == "" {
		return
	}
	if c.handle == nil || c.handleMode != progressModeSpinner {
		if c.handle != nil {
			c.handle.Clear()
		}
		c.handle = newSpinnerProgress(c.cmd, description)
		c.handleMode = progressModeSpinner
		c.downloaded = 0
		c.downloadTotal = 0
		return
	}
	c.handle.Describe(description)
}

func (c *singleManagedApplyController) updateDownloadProgress(downloaded, total int64) {
	description := fmt.Sprintf("Downloading %s", c.appID)
	if downloaded <= 0 && total == 0 {
		c.setSpinnerDescription(description)
		return
	}
	if c.handle == nil || c.handleMode != progressModeBytes || c.downloadTotal != total {
		if c.handle != nil {
			c.handle.Clear()
		}
		c.handle = newByteProgress(c.cmd, description, total)
		c.handleMode = progressModeBytes
		c.downloaded = 0
		c.downloadTotal = total
	}

	c.handle.Describe(description)
	delta := downloaded - c.downloaded
	if delta > 0 {
		c.handle.Add(delta)
		c.downloaded = downloaded
	}
}

func completedCount(completed []bool) int {
	total := 0
	for _, value := range completed {
		if value {
			total++
		}
	}
	return total
}

func activeCount(total, completed int) int {
	active := total - completed
	if active < 0 {
		return 0
	}
	return active
}

func managedApplyAggregateDescription(total, completed, failures int) string {
	active := activeCount(total, completed)
	return fmt.Sprintf("Updating apps (%d/%d complete, %d failed, %d active)", completed, total, failures, active)
}
