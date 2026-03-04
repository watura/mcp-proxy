package proxy

import (
	"context"
	"fmt"
	"log/slog"

	"git.wtr.app/watura/mcp-proxy/internal/backend"
	"git.wtr.app/watura/mcp-proxy/internal/config"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// ProxySession holds backend connections shared across all clients.
type ProxySession struct {
	Backends map[string]*backend.Backend // serverName -> backend
	Caps     *AggregatedCapabilities
}

// Close shuts down all backend connections.
func (ps *ProxySession) Close() {
	for name, b := range ps.Backends {
		if err := b.Close(); err != nil {
			slog.Warn("error closing backend", "server", name, "error", err)
		}
	}
}

// CreateSharedSession creates backend connections, discovers capabilities, and returns
// a session holding both the persistent connections and aggregated capabilities.
func CreateSharedSession(ctx context.Context, cfg *config.Config) (*ProxySession, error) {
	backends := make(map[string]*backend.Backend)
	var serverCaps []ServerCapabilities

	for name, sc := range cfg.MCPServers {
		slog.Info("connecting backend", "server", name)

		b, err := backend.NewBackend(ctx, name, sc)
		if err != nil {
			slog.Error("failed to create backend", "server", name, "error", err)
			continue
		}

		initResult, err := b.Initialize(ctx)
		if err != nil {
			slog.Error("failed to initialize backend", "server", name, "error", err)
			b.Close()
			continue
		}

		caps := ServerCapabilities{Name: name}

		if initResult.Capabilities.Tools != nil {
			tools, err := ListBackendTools(ctx, b.Client)
			if err != nil {
				slog.Warn("failed to list tools", "server", name, "error", err)
			} else {
				caps.Tools = tools
			}
		}

		if initResult.Capabilities.Resources != nil {
			resources, err := ListBackendResources(ctx, b.Client)
			if err != nil {
				slog.Warn("failed to list resources", "server", name, "error", err)
			} else {
				caps.Resources = resources
			}
		}

		if initResult.Capabilities.Prompts != nil {
			prompts, err := ListBackendPrompts(ctx, b.Client)
			if err != nil {
				slog.Warn("failed to list prompts", "server", name, "error", err)
			} else {
				caps.Prompts = prompts
			}
		}

		backends[name] = b
		serverCaps = append(serverCaps, caps)
		slog.Info("backend ready", "server", name,
			"tools", len(caps.Tools),
			"resources", len(caps.Resources),
			"prompts", len(caps.Prompts))
	}

	if len(backends) == 0 {
		return nil, fmt.Errorf("no backends could be started")
	}

	aggregated := Aggregate(serverCaps)
	slog.Info("shared session created",
		"backends", len(backends),
		"tools", len(aggregated.Tools),
		"resources", len(aggregated.Resources),
		"prompts", len(aggregated.Prompts))

	return &ProxySession{Backends: backends, Caps: aggregated}, nil
}

// ListBackendTools lists tools from a specific backend.
func ListBackendTools(ctx context.Context, c client.MCPClient) ([]mcp.Tool, error) {
	result, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// ListBackendResources lists resources from a specific backend.
func ListBackendResources(ctx context.Context, c client.MCPClient) ([]mcp.Resource, error) {
	result, err := c.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		return nil, err
	}
	return result.Resources, nil
}

// ListBackendPrompts lists prompts from a specific backend.
func ListBackendPrompts(ctx context.Context, c client.MCPClient) ([]mcp.Prompt, error) {
	result, err := c.ListPrompts(ctx, mcp.ListPromptsRequest{})
	if err != nil {
		return nil, err
	}
	return result.Prompts, nil
}
