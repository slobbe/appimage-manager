package main

import "github.com/spf13/cobra"

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
	indicator := newBusyIndicator(cmd, label)
	indicator.Start()
	defer indicator.Stop()
	return fn()
}
