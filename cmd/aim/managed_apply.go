package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	models "github.com/slobbe/appimage-manager/internal/types"
	"github.com/spf13/cobra"
)

const maxManagedApplyWorkers = 5

var managedApplyRenderInterval = 120 * time.Millisecond

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

type managedApplyRowState struct {
	appID         string
	stage         managedApplyStage
	downloaded    int64
	downloadTotal int64
	version       string
	message       string
}

type managedApplyRenderer struct {
	cmd      *cobra.Command
	total    int
	tty      bool
	rows     []managedApplyRowState
	mu       sync.Mutex
	drawn    bool
	dirty    bool
	header   string
	stopCh   chan struct{}
	stopOnce sync.Once
	doneCh   chan struct{}
}

func runManagedApplies(ctx context.Context, cmd *cobra.Command, pending []pendingManagedUpdate) []managedApplyResult {
	if len(pending) == 0 {
		return nil
	}

	renderer := newManagedApplyRenderer(cmd, pending)
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
					renderer.Event(event)
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

	renderer.Finish(results)
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

func newManagedApplyRenderer(cmd *cobra.Command, pending []pendingManagedUpdate) *managedApplyRenderer {
	rows := make([]managedApplyRowState, len(pending))
	for idx, item := range pending {
		appID := ""
		if item.App != nil {
			appID = item.App.ID
		}
		rows[idx] = managedApplyRowState{
			appID: appID,
			stage: managedApplyStageQueued,
		}
	}

	renderer := &managedApplyRenderer{
		cmd:    cmd,
		total:  len(pending),
		tty:    isTerminalOutput(),
		rows:   rows,
		header: managedApplyHeader(len(pending)),
	}

	if strings.TrimSpace(renderer.header) != "" {
		printInfo(cmd, renderer.header)
	}
	if renderer.tty {
		renderer.dirty = false
		renderer.renderLocked()
		renderer.startRenderLoop()
	}

	return renderer
}

func managedApplyHeader(total int) string {
	if total == 1 {
		return "Applying 1 update"
	}
	return ""
}

func (r *managedApplyRenderer) Event(event managedApplyEvent) {
	if event.Index < 0 || event.Index >= len(r.rows) {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	row := &r.rows[event.Index]
	if strings.TrimSpace(event.AppID) != "" {
		row.appID = strings.TrimSpace(event.AppID)
	}
	if event.Stage != "" {
		row.stage = event.Stage
	}
	row.downloaded = event.Downloaded
	row.downloadTotal = event.DownloadTotal
	if strings.TrimSpace(event.Version) != "" {
		row.version = strings.TrimSpace(event.Version)
	}
	if strings.TrimSpace(event.Message) != "" {
		row.message = strings.TrimSpace(event.Message)
	}
	r.dirty = true
}

func (r *managedApplyRenderer) Finish(results []managedApplyResult) {
	r.stopRenderLoop()

	r.mu.Lock()
	for idx, result := range results {
		if idx < 0 || idx >= len(r.rows) {
			continue
		}

		row := &r.rows[idx]
		if result.app != nil && strings.TrimSpace(row.appID) == "" {
			row.appID = result.app.ID
		}
		if result.err != nil {
			row.stage = managedApplyStageFailed
			row.message = result.err.Error()
			continue
		}
		if result.updatedApp != nil {
			row.stage = managedApplyStageDone
			row.version = result.updatedApp.Version
		}
	}
	r.dirty = true

	if r.tty {
		r.renderLocked()
	}

	rows := make([]string, len(r.rows))
	if !r.tty {
		for idx := range r.rows {
			rows[idx] = r.formatRow(idx)
		}
	}
	r.mu.Unlock()

	if !r.tty {
		for _, row := range rows {
			fmt.Println(row)
		}
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
			printError(r.cmd, fmt.Sprintf("Failed: %s: %v", appID, result.err))
			continue
		}
		if result.updatedApp != nil {
			successes++
		}
	}

	summary := fmt.Sprintf("Updated %d app(s); %d failed", successes, failures)
	if failures > 0 {
		printWarning(r.cmd, summary)
		return
	}
	printSuccess(r.cmd, summary)
}

func (r *managedApplyRenderer) startRenderLoop() {
	if !r.tty {
		return
	}

	r.stopCh = make(chan struct{})
	r.doneCh = make(chan struct{})

	go func() {
		ticker := time.NewTicker(managedApplyRenderInterval)
		defer ticker.Stop()
		defer close(r.doneCh)

		for {
			select {
			case <-ticker.C:
				r.renderIfDirty()
			case <-r.stopCh:
				return
			}
		}
	}()
}

func (r *managedApplyRenderer) stopRenderLoop() {
	if !r.tty || r.stopCh == nil {
		return
	}

	r.stopOnce.Do(func() {
		close(r.stopCh)
		<-r.doneCh
	})
}

func (r *managedApplyRenderer) renderIfDirty() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.dirty {
		return
	}

	r.renderLocked()
	r.dirty = false
}

func (r *managedApplyRenderer) renderLocked() {
	if !r.tty {
		return
	}

	if r.drawn {
		fmt.Printf("\033[%dA", len(r.rows))
	}
	for idx := range r.rows {
		fmt.Printf("\033[2K\r%s\n", r.formatRow(idx))
	}
	r.drawn = true
}

func (r *managedApplyRenderer) formatRow(index int) string {
	row := r.rows[index]
	status := managedApplyStatusText(row)
	if r.tty {
		status = colorize(true, managedApplyColorCode(row.stage), status)
	}

	appID := row.appID
	if strings.TrimSpace(appID) == "" {
		appID = "<unknown>"
	}
	return fmt.Sprintf("[%d/%d] %s %s", index+1, r.total, appID, status)
}

func managedApplyStatusText(row managedApplyRowState) string {
	switch row.stage {
	case managedApplyStageQueued:
		return "queued"
	case managedApplyStageZsync:
		return "delta update"
	case managedApplyStageDownload:
		return formatManagedDownloadStatus(row.downloaded, row.downloadTotal)
	case managedApplyStageVerify:
		return "verifying"
	case managedApplyStageIntegrate:
		return "integrating"
	case managedApplyStageDone:
		return "updated -> " + displayVersion(row.version)
	case managedApplyStageFailed:
		return "failed"
	default:
		return "queued"
	}
}

func formatManagedDownloadStatus(downloaded, total int64) string {
	if total > 0 {
		percent := float64(downloaded) / float64(total)
		if percent < 0 {
			percent = 0
		}
		if percent > 1 {
			percent = 1
		}
		return fmt.Sprintf("downloading %.1f%% (%s/%s)", percent*100, formatByteSize(downloaded), formatByteSize(total))
	}
	return fmt.Sprintf("downloading %s", formatByteSize(downloaded))
}

func managedApplyColorCode(stage managedApplyStage) string {
	switch stage {
	case managedApplyStageDone:
		return "\033[0;32m"
	case managedApplyStageFailed:
		return "\033[0;31m"
	case managedApplyStageQueued:
		return "\033[0;33m"
	default:
		return "\033[0;36m"
	}
}
