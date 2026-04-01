package main

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

type operationLogKey struct{}

type operationLogBuffer struct {
	mu    sync.Mutex
	lines []string
}

func withOperationLog(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if operationLogFromContext(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, operationLogKey{}, &operationLogBuffer{})
}

func operationLogFromContext(ctx context.Context) *operationLogBuffer {
	if ctx == nil {
		return nil
	}
	if value, ok := ctx.Value(operationLogKey{}).(*operationLogBuffer); ok {
		return value
	}
	return nil
}

func operationLogForCommand(cmd *cobra.Command) *operationLogBuffer {
	if cmd == nil {
		return nil
	}
	return operationLogFromContext(cmd.Context())
}

func logOperationf(cmd *cobra.Command, format string, args ...interface{}) {
	buffer := operationLogForCommand(cmd)
	if buffer == nil {
		return
	}
	buffer.Logf(format, args...)
}

func logOperationContextf(ctx context.Context, format string, args ...interface{}) {
	buffer := operationLogFromContext(ctx)
	if buffer == nil {
		return
	}
	buffer.Logf(format, args...)
}

func (b *operationLogBuffer) Logf(format string, args ...interface{}) {
	if b == nil {
		return
	}
	line := strings.TrimSpace(fmt.Sprintf(format, args...))
	if line == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = append(b.lines, line)
}

func (b *operationLogBuffer) Lines() []string {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	lines := make([]string, len(b.lines))
	copy(lines, b.lines)
	return lines
}
