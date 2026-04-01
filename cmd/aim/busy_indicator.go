package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
)

var busyIndicatorRenderInterval = 120 * time.Millisecond

var busyIndicatorFrames = []string{"|", "/", "-", "\\"}

type busyIndicator struct {
	cmd      *cobra.Command
	label    string
	tty      bool
	frameIdx int
	width    int
	stopCh   chan struct{}
	doneCh   chan struct{}
	stopOnce sync.Once
}

func newBusyIndicator(cmd *cobra.Command, label string) *busyIndicator {
	return &busyIndicator{
		cmd:    cmd,
		label:  strings.TrimSpace(label),
		tty:    isTerminalOutput(),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

func (b *busyIndicator) Start() {
	if b == nil || b.label == "" {
		return
	}

	if !b.tty {
		writeLogf(b.cmd, "%s...\n", b.label)
		return
	}

	b.render()
	go b.renderLoop()
}

func (b *busyIndicator) Stop() {
	if b == nil || b.label == "" {
		return
	}

	if !b.tty {
		return
	}

	b.stopOnce.Do(func() {
		close(b.stopCh)
		<-b.doneCh
		b.clear()
	})
}

func (b *busyIndicator) renderLoop() {
	ticker := time.NewTicker(busyIndicatorRenderInterval)
	defer func() {
		ticker.Stop()
		close(b.doneCh)
	}()

	for {
		select {
		case <-ticker.C:
			b.render()
		case <-b.stopCh:
			return
		}
	}
}

func (b *busyIndicator) render() {
	frame := busyIndicatorFrames[b.frameIdx%len(busyIndicatorFrames)]
	b.frameIdx++

	line := colorize(useColor(b.cmd), "\033[0;36m", fmt.Sprintf("%s %s", frame, b.label))
	width := visibleWidth(frame) + 1 + visibleWidth(b.label)
	if width < b.width {
		line += strings.Repeat(" ", b.width-width)
		width = b.width
	}
	b.width = width

	writeLogf(b.cmd, "\r%s", line)
}

func (b *busyIndicator) clear() {
	if b.width <= 0 {
		return
	}

	writeLogf(b.cmd, "\r%s\r", strings.Repeat(" ", b.width))
}

func runWithBusyIndicator[T any](cmd *cobra.Command, label string, fn func() (T, error)) (T, error) {
	indicator := newBusyIndicator(cmd, label)
	indicator.Start()
	defer indicator.Stop()
	return fn()
}

func visibleWidth(value string) int {
	return len([]rune(value))
}
