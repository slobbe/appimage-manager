package app

import "context"

// WorkspaceProvider creates temporary workspaces for use cases that need
// intermediate filesystem state.
type WorkspaceProvider interface {
	Create(ctx context.Context) (Workspace, error)
}

// Workspace is a temporary directory and its cleanup function.
type Workspace struct {
	Path    string
	Cleanup func() error
}
