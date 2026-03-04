package proxy

import (
	"context"

	"git.wtr.app/watura/mcp-proxy/internal/backend"
	"git.wtr.app/watura/mcp-proxy/internal/config"
)

// newBootstrapBackend creates a temporary backend for capability discovery.
func newBootstrapBackend(ctx context.Context, name string, sc *config.ServerConfig) (*backend.Backend, error) {
	return backend.NewBackend(ctx, name, sc)
}
