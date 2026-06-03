package cli

import (
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

var busyIndicatorRenderInterval = progressThrottleInterval

type busyIndicator struct {
	cmd    *cobra.Command
	label  string
	handle progressHandle
}

func newBusyIndicator(cmd *cobra.Command, label string) *busyIndicator {
	return &busyIndicator{
		cmd:   cmd,
		label: label,
	}
}

func (b *busyIndicator) Start() {
	if b == nil || b.handle != nil {
		return
	}
	b.handle = newSpinnerProgress(b.cmd, b.label)
}

func (b *busyIndicator) Stop() {
	if b == nil || b.handle == nil {
		return
	}
	b.handle.Clear()
	b.handle = nil
}

func runWithBusyIndicator[T any](cmd *cobra.Command, label string, fn func() (T, error)) (T, error) {
	logOperationf(cmd, "%s", label)
	indicator := newBusyIndicator(cmd, label)
	indicator.Start()
	defer indicator.Stop()
	return fn()
}

const progressThrottleInterval = 65 * time.Millisecond

type progressHandle interface {
	Describe(string)
	Add(int64)
	Finish()
	Clear()
}

type progressMode int

const (
	progressModeSpinner progressMode = iota
	progressModeBytes
	progressModeCount
)

var spinnerFrames = []string{"|", "/", "-", `\`}

type ttyProgressHandle struct {
	writer         io.Writer
	mode           progressMode
	description    string
	current        int64
	total          int64
	lastRenderedAt time.Time
	lastLineWidth  int
	lastText       string
	spinnerIndex   int
	stopped        bool
	stopCh         chan struct{}
	doneCh         chan struct{}
	mu             sync.Mutex
}

func (h *ttyProgressHandle) Describe(text string) {
	if h == nil {
		return
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped || text == h.description {
		return
	}
	h.description = text
	h.renderLocked(true)
}

func (h *ttyProgressHandle) Add(delta int64) {
	if h == nil || delta == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped {
		return
	}
	h.current += delta
	if h.current < 0 {
		h.current = 0
	}
	h.renderLocked(false)
}

func (h *ttyProgressHandle) Finish() { h.stopAndClear() }

func (h *ttyProgressHandle) Clear() { h.stopAndClear() }

func (h *ttyProgressHandle) stopAndClear() {
	if h == nil {
		return
	}

	done := func() chan struct{} {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.stopped {
			return nil
		}
		h.stopped = true
		if h.stopCh == nil {
			return nil
		}
		close(h.stopCh)
		return h.doneCh
	}()

	if done != nil {
		<-done
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopCh = nil
	h.clearLocked()
}

func (h *ttyProgressHandle) renderLocked(force bool) {
	if h.stopped {
		return
	}

	text := h.renderTextLocked()
	if text == "" {
		return
	}

	now := time.Now()
	if !force && !h.lastRenderedAt.IsZero() && now.Sub(h.lastRenderedAt) < progressThrottleInterval {
		return
	}
	if !force && text == h.lastText {
		return
	}

	width := visibleWidth(text)
	if width < h.lastLineWidth {
		text += strings.Repeat(" ", h.lastLineWidth-width)
		width = h.lastLineWidth
	}

	_, _ = fmt.Fprintf(h.writer, "\r%s", text)
	h.lastRenderedAt = now
	h.lastLineWidth = width
	h.lastText = text
}

func (h *ttyProgressHandle) renderTextLocked() string {
	description := strings.TrimSpace(h.description)
	switch h.mode {
	case progressModeSpinner:
		frame := spinnerFrames[h.spinnerIndex%len(spinnerFrames)]
		return fmt.Sprintf("%s %s", frame, description)
	case progressModeBytes:
		switch {
		case h.total > 0:
			return fmt.Sprintf("%s %s/%s", description, formatByteSize(h.current), formatByteSize(h.total))
		case h.current > 0:
			return fmt.Sprintf("%s %s", description, formatByteSize(h.current))
		default:
			return description
		}
	case progressModeCount:
		if description != "" {
			return description
		}
		if h.total > 0 {
			return fmt.Sprintf("%d/%d", h.current, h.total)
		}
		return fmt.Sprintf("%d", h.current)
	default:
		return description
	}
}

func (h *ttyProgressHandle) clearLocked() {
	if h.lastLineWidth <= 0 {
		return
	}
	_, _ = fmt.Fprintf(h.writer, "\r%s\r", strings.Repeat(" ", h.lastLineWidth))
	h.lastLineWidth = 0
	h.lastText = ""
}

func (h *ttyProgressHandle) spin() {
	ticker := time.NewTicker(progressThrottleInterval)
	defer func() {
		ticker.Stop()
		close(h.doneCh)
	}()

	for {
		select {
		case <-ticker.C:
			h.mu.Lock()
			if h.stopped {
				h.mu.Unlock()
				return
			}
			h.spinnerIndex = (h.spinnerIndex + 1) % len(spinnerFrames)
			h.renderLocked(true)
			h.mu.Unlock()
		case <-h.stopCh:
			return
		}
	}
}

type plainProgressHandle struct {
	writer      io.Writer
	description string
	printed     bool
}

func (h *plainProgressHandle) Describe(text string) {
	if h == nil {
		return
	}
	h.description = strings.TrimSpace(text)
}

func (h *plainProgressHandle) Add(int64) {}

func (h *plainProgressHandle) Finish() {}

func (h *plainProgressHandle) Clear() {}

func newSpinnerProgress(cmd *cobra.Command, description string) progressHandle {
	return newCommandProgressHandle(cmd, progressModeSpinner, strings.TrimSpace(description), -1)
}

func newByteProgress(cmd *cobra.Command, description string, total int64) progressHandle {
	return newCommandProgressHandle(cmd, progressModeBytes, strings.TrimSpace(description), total)
}

func newCountProgress(cmd *cobra.Command, description string, total int64) progressHandle {
	return newCommandProgressHandle(cmd, progressModeCount, strings.TrimSpace(description), total)
}

func newProcessSpinnerProgress(description string, interactive bool) progressHandle {
	return newRawProgressHandle(os.Stderr, interactive, true, progressModeSpinner, strings.TrimSpace(description), -1)
}

func newProcessByteProgress(description string, total int64, interactive bool) progressHandle {
	return newRawProgressHandle(os.Stderr, interactive, true, progressModeBytes, strings.TrimSpace(description), total)
}

func newCommandProgressHandle(cmd *cobra.Command, mode progressMode, description string, total int64) progressHandle {
	opts := runtimeOptionsFrom(cmd)
	if opts.Quiet {
		return nil
	}
	return newRawProgressHandle(logWriter(cmd), isTerminalStderr(), true, mode, description, total)
}

func newRawProgressHandle(writer io.Writer, interactive bool, enabled bool, mode progressMode, description string, total int64) progressHandle {
	description = strings.TrimSpace(description)
	if !enabled || description == "" {
		return nil
	}

	if !interactive {
		handle := &plainProgressHandle{
			writer:      writer,
			description: description,
		}
		fmt.Fprintf(writer, "%s...\n", description)
		handle.printed = true
		return handle
	}

	handle := &ttyProgressHandle{
		writer:      writer,
		mode:        mode,
		description: description,
		total:       total,
	}
	if mode == progressModeSpinner {
		handle.stopCh = make(chan struct{})
		handle.doneCh = make(chan struct{})
	}

	handle.mu.Lock()
	handle.renderLocked(true)
	handle.mu.Unlock()

	if mode == progressModeSpinner {
		go handle.spin()
	}

	return handle
}

func visibleWidth(value string) int {
	return len([]rune(value))
}

const (
	sectionApp      = "App"
	sectionAppImage = "AppImage"
	sectionUpdates  = "Updates"
	sectionState    = "State"
)

func progressCheckAimUpdates() string {
	return "Checking for aim updates"
}

func progressUpgradeAim() string {
	return "Upgrading aim"
}

func warningNoEmbeddedSource() string {
	return "No embedded update source found in the current AppImage"
}

func formatPrompt(action, target string) string {
	return fmt.Sprintf("%s %s? [y/N]: ", action, target)
}

func colorize(enabled bool, code, value string) string {
	if !enabled {
		return value
	}

	return code + value + "\033[0m"
}

func printSuccess(cmd *cobra.Command, text string) {
	if runtimeOptionsFrom(cmd).Quiet || !shouldRenderLogs(cmd) {
		return
	}
	writeLogf(cmd, "%s\n", colorize(shouldColorStderr(cmd), "\033[0;32m", text))
}

func printWarning(cmd *cobra.Command, text string) {
	if !shouldRenderLogs(cmd) {
		return
	}
	writeLogf(cmd, "%s\n", colorize(shouldColorStderr(cmd), "\033[0;33m", text))
}

func printError(cmd *cobra.Command, text string) {
	writeLogf(cmd, "%s\n", colorize(shouldColorStderr(cmd), "\033[0;31m", text))
}

func printInfo(cmd *cobra.Command, text string) {
	if runtimeOptionsFrom(cmd).Quiet || !shouldRenderLogs(cmd) {
		return
	}
	writeLogf(cmd, "%s\n", colorize(shouldColorStderr(cmd), "\033[0;36m", text))
}

func printSection(cmd *cobra.Command, text string) {
	if shouldUseStructuredOutput(cmd) {
		return
	}
	writeDataf(cmd, "%s\n", colorize(shouldColorStdout(cmd), "\033[1m", text))
}

func printCurrentIncoming(cmd *cobra.Command, current, incoming string) {
	writeLogf(cmd, "Current:\n")
	writeLogf(cmd, "  %s\n", current)
	writeLogf(cmd, "Incoming:\n")
	writeLogf(cmd, "  %s\n", incoming)
}

func printCurrentValue(cmd *cobra.Command, current string) {
	writeLogf(cmd, "Current:\n")
	writeLogf(cmd, "  %s\n", current)
}
