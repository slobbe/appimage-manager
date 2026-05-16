package update

import (
	"context"
	"strings"
	"sync"

	models "github.com/slobbe/appimage-manager/internal/domain"
)

const MaxManagedApplyWorkers = 5

type ManagedApplyResult struct {
	Index      int
	App        *models.App
	UpdatedApp *models.App
	Error      error
}

type ManagedApplyFunc func(ctx context.Context, update ManagedUpdate, reporter ManagedApplyReporter) (*models.App, error)

type ManagedApplyReporterFactory func(index, total int, update ManagedUpdate) ManagedApplyReporter

func ApplyManagedUpdates(ctx context.Context, pending []ManagedUpdate, apply ManagedApplyFunc, reporterFor ManagedApplyReporterFactory) []ManagedApplyResult {
	if len(pending) == 0 {
		return nil
	}
	if apply == nil {
		service := NewService(Service{})
		apply = service.ApplyManagedUpdate
	}

	results := make([]ManagedApplyResult, len(pending))
	jobs := make(chan int, len(pending))
	workerCount := ManagedApplyWorkerCount(len(pending))

	var wg sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				item := pending[idx]
				reporter := ManagedApplyReporter(nil)
				if reporterFor != nil {
					reporter = reporterFor(idx, len(pending), item)
				}
				updatedApp, err := apply(ctx, item, reporter)
				results[idx] = ManagedApplyResult{Index: idx, App: item.App, UpdatedApp: updatedApp, Error: err}
			}
		}()
	}

	for idx := range pending {
		jobs <- idx
	}
	close(jobs)
	wg.Wait()
	return results
}

func ManagedApplyWorkerCount(total int) int {
	if total <= 0 {
		return 0
	}
	if total < MaxManagedApplyWorkers {
		return total
	}
	return MaxManagedApplyWorkers
}

func WithManagedApplyEventDefaults(reporter ManagedApplyReporter, index, total int, update ManagedUpdate) ManagedApplyReporter {
	return ManagedApplyReporterFunc(func(event ManagedApplyEvent) {
		if update.App != nil && strings.TrimSpace(event.AppID) == "" {
			event.AppID = update.App.ID
		}
		event.Index = index
		event.Total = total
		if reporter != nil {
			reporter.Event(event)
		}
	})
}
