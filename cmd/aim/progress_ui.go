package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

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

type progressbarHandle struct {
	bar  *progressbar.ProgressBar
	mode progressMode
}

func (h *progressbarHandle) Describe(text string) {
	if h == nil || h.bar == nil {
		return
	}
	h.bar.Describe(strings.TrimSpace(text))
}

func (h *progressbarHandle) Add(delta int64) {
	if h == nil || h.bar == nil || delta == 0 {
		return
	}
	_ = h.bar.Add64(delta)
}

func (h *progressbarHandle) Finish() {
	if h == nil || h.bar == nil {
		return
	}
	_ = h.bar.Finish()
}

func (h *progressbarHandle) Clear() {
	if h == nil || h.bar == nil {
		return
	}
	_ = h.bar.Clear()
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

	options := []progressbar.Option{
		progressbar.OptionSetWriter(writer),
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionFullWidth(),
		progressbar.OptionThrottle(progressThrottleInterval),
		progressbar.OptionUseANSICodes(true),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionSetWidth(10),
	}

	switch mode {
	case progressModeBytes:
		options = append(options,
			progressbar.OptionShowBytes(true),
			progressbar.OptionShowCount(),
			progressbar.OptionShowTotalBytes(true),
		)
	case progressModeCount:
		options = append(options,
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
		)
	default:
		options = append(options,
			progressbar.OptionShowCount(),
			progressbar.OptionShowIts(),
		)
	}

	max := total
	if mode == progressModeSpinner || max <= 0 {
		max = -1
	}

	bar := progressbar.NewOptions64(max, options...)
	return &progressbarHandle{bar: bar, mode: mode}
}
