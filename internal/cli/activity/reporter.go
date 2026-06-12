package activity

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aim/internal/app"

	"golang.org/x/sys/unix"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type Reporter struct {
	enabled bool
	w       io.Writer

	mu       sync.Mutex
	tasks    []*task
	lines    int
	frame    int
	interval time.Duration

	done chan struct{}
	once sync.Once
	wg   sync.WaitGroup
}

func NewReporter(w io.Writer, enabled bool) *Reporter {
	r := &Reporter{
		enabled:  enabled,
		w:        w,
		interval: 100 * time.Millisecond,
		done:     make(chan struct{}),
	}

	if enabled {
		r.start()
	}

	return r
}

func (r *Reporter) Start(_ context.Context, activity app.Activity) app.ActivityTask {
	if !r.enabled {
		return noopTask{}
	}

	t := &task{
		reporter: r,
		activity: activity,
		total:    activity.Total,
		unit:     activity.Unit,
	}

	r.mu.Lock()
	r.tasks = append(r.tasks, t)
	r.mu.Unlock()

	return t
}

func (r *Reporter) Wait() {
	if !r.enabled {
		return
	}

	r.once.Do(func() {
		close(r.done)
		r.wg.Wait()

		r.mu.Lock()
		defer r.mu.Unlock()
		r.clearLocked()
	})
}

func (r *Reporter) start() {
	r.wg.Add(1)

	go func() {
		defer r.wg.Done()

		r.render()

		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		for {
			select {
			case <-r.done:
				return
			case <-ticker.C:
				r.render()
			}
		}
	}()
}

func (r *Reporter) render() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.clearLocked()

	active := r.activeTasksLocked()
	if len(active) == 0 {
		return
	}

	width := terminalWidth(r.w)
	if width <= 0 {
		width = 80
	}

	layout := progressLayoutFor(active, width)
	for _, t := range active {
		fmt.Fprintln(r.w, r.renderTaskLocked(t, width, layout))
	}

	r.lines = len(active)
	r.frame++
}

func (r *Reporter) activeTasksLocked() []*task {
	active := make([]*task, 0, len(r.tasks))

	for _, t := range r.tasks {
		t.mu.Lock()
		finished := t.finished
		t.mu.Unlock()

		if !finished {
			active = append(active, t)
		}
	}

	return active
}

func (r *Reporter) renderTaskLocked(t *task, width int, layout progressLayout) string {
	t.mu.Lock()
	defer t.mu.Unlock()

	title := activityTitle(t.activity)
	if t.total > 0 {
		return fit(renderProgress(title, t.message, t.current, t.total, t.unit, width, layout), width)
	}

	frame := spinnerFrames[r.frame%len(spinnerFrames)]
	line := frame + " " + title
	if t.message != "" {
		line += " - " + t.message
	}

	return fit(line, width)
}

func activityTitle(activity app.Activity) string {
	switch activity.Kind {
	case app.ActivityKindCheckingGitHub:
		return "Checking " + activity.Repo + " on GitHub ..."
	case app.ActivityKindIntegrating:
		return "Integrating " + filepath.Base(activity.Path)
	case app.ActivityKindRemoving:
		return "Removing " + activity.AppID + " ..."
	case app.ActivityKindCheckingUpdates:
		return "Checking for updates ..."
	case app.ActivityKindWaiting:
		return "[" + activity.AppID + "]"
	case app.ActivityKindDownloading:
		if activity.AppID != "" {
			return "[" + activity.AppID + "] Downloading " + activity.AssetName
		}
		return "Downloading " + activity.AssetName
	default:
		return "Working ..."
	}
}

type progressLayout struct {
	prefixWidth   int
	barWidth      int
	progressWidth int
}

func progressLayoutFor(tasks []*task, width int) progressLayout {
	layout := progressLayout{}

	for _, task := range tasks {
		task.mu.Lock()
		if task.total <= 0 {
			task.mu.Unlock()
			continue
		}

		label := activityTitle(task.activity)
		if task.message != "" {
			label += " - " + task.message
		}

		_, progressWidth := progressLabel(task.current, task.total, task.unit)
		layout.prefixWidth = max(layout.prefixWidth, runeLen(label)+1)
		layout.progressWidth = max(layout.progressWidth, progressWidth)
		task.mu.Unlock()
	}

	if layout.prefixWidth == 0 {
		return layout
	}

	layout.barWidth = width - layout.prefixWidth - 1 - layout.progressWidth - 2
	return layout
}

func renderProgress(title string, message string, current int64, total int64, unit app.ActivityUnit, width int, layout progressLayout) string {
	if total <= 0 {
		return title
	}

	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}

	percent := float64(current) / float64(total)
	progressText, progressWidth := progressLabel(current, total, unit)

	label := title
	if message != "" {
		label += " - " + message
	}

	prefix := label + " "
	prefixWidth := runeLen(prefix)
	barWidth := width - prefixWidth - 1 - progressWidth - 2
	if layout.barWidth > 0 {
		prefixWidth = layout.prefixWidth
		progressWidth = layout.progressWidth
		barWidth = layout.barWidth
	}

	suffix := " " + fmt.Sprintf("%*s", progressWidth, progressText)
	if barWidth < 1 {
		return label + " " + progressText
	}

	filled := int(percent * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	bar := "[" + strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled) + "]"
	return fmt.Sprintf("%-*s", prefixWidth, prefix) + bar + suffix
}

func (r *Reporter) clearLocked() {
	if r.lines == 0 {
		return
	}

	fmt.Fprintf(r.w, "\x1b[%dA", r.lines)
	fmt.Fprint(r.w, "\x1b[J")
	r.lines = 0
}

func terminalWidth(w io.Writer) int {
	file, ok := w.(*os.File)
	if !ok {
		return 0
	}

	size, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ)
	if err != nil || size.Col == 0 {
		return 0
	}

	if size.Col <= 1 {
		return int(size.Col)
	}

	return int(size.Col) - 1
}

func fit(s string, width int) string {
	if width <= 0 {
		return s
	}

	runes := []rune(s)
	if len(runes) <= width {
		return s
	}

	if width <= 1 {
		return string(runes[:width])
	}

	return string(runes[:width-1]) + "…"
}

func runeLen(s string) int {
	return len([]rune(s))
}

func progressLabel(current int64, total int64, unit app.ActivityUnit) (string, int) {
	if unit == app.ActivityUnitBytes {
		label := fmt.Sprintf("%s/%s", formatBytes(current), formatBytes(total))
		maxLabel := fmt.Sprintf("%s/%s", formatBytes(total), formatBytes(total))
		return label, runeLen(maxLabel)
	}

	label := fmt.Sprintf("%3.0f%%", float64(current)/float64(total)*100)
	return label, len("100%")
}

func formatBytes(bytes int64) string {
	const mb = 1024 * 1024
	if bytes < mb {
		return fmt.Sprintf("%d B", bytes)
	}

	return fmt.Sprintf("%d MB", bytes/mb)
}

type task struct {
	mu       sync.Mutex
	reporter *Reporter
	activity app.Activity
	total    int64
	current  int64
	unit     app.ActivityUnit
	message  string
	finished bool
	failed   bool
}

func (t *task) Message(message string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.finished {
		return
	}

	t.message = message
}

func (t *task) Advance(delta int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.finished {
		return
	}

	t.current += delta
	if t.current < 0 {
		t.current = 0
	}
	if t.total > 0 && t.current > t.total {
		t.current = t.total
	}
}

func (t *task) Set(current int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.finished {
		return
	}

	t.current = current
	if t.current < 0 {
		t.current = 0
	}
	if t.total > 0 && t.current > t.total {
		t.current = t.total
	}
}

func (t *task) Done(message string) {
	t.mu.Lock()

	if t.finished {
		t.mu.Unlock()
		return
	}

	t.message = message
	t.finished = true
	reporter := t.reporter
	t.mu.Unlock()

	if reporter != nil {
		reporter.render()
	}
}

func (t *task) Fail(err error) {
	t.mu.Lock()

	if t.finished {
		t.mu.Unlock()
		return
	}

	if err != nil {
		t.message = err.Error()
	}
	t.failed = true
	t.finished = true
	reporter := t.reporter
	t.mu.Unlock()

	if reporter != nil {
		reporter.render()
	}
}

type noopTask struct{}

func (noopTask) Message(message string) {}
func (noopTask) Advance(delta int64)    {}
func (noopTask) Set(current int64)      {}
func (noopTask) Done(message string)    {}
func (noopTask) Fail(err error)         {}
