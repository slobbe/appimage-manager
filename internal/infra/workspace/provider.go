package workspace

import (
	"context"
	"fmt"
	"os"

	"aim/internal/app"
)

const defaultPattern = "aim-*"

// Provider creates temporary filesystem workspaces.
type Provider struct {
	BaseDir string
	Pattern string
}

// NewProvider creates a workspace provider rooted at baseDir. If baseDir is
// empty, the operating system default temp directory is used.
func NewProvider(baseDir string) Provider {
	return Provider{BaseDir: baseDir, Pattern: defaultPattern}
}

var _ app.WorkspaceProvider = Provider{}

// Create creates a temporary workspace directory.
func (p Provider) Create(ctx context.Context) (app.Workspace, error) {
	if err := ctx.Err(); err != nil {
		return app.Workspace{}, err
	}

	pattern := p.Pattern
	if pattern == "" {
		pattern = defaultPattern
	}

	path, err := os.MkdirTemp(p.BaseDir, pattern)
	if err != nil {
		return app.Workspace{}, fmt.Errorf("create workspace: %w", err)
	}
	if err := ctx.Err(); err != nil {
		_ = os.RemoveAll(path)
		return app.Workspace{}, err
	}

	return app.Workspace{
		Path: path,
		Cleanup: func() error {
			return os.RemoveAll(path)
		},
	}, nil
}
