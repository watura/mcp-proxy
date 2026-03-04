package backend

import (
	"context"
	"fmt"

	"git.wtr.app/watura/mcp-proxy/internal/config"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Backend wraps an MCP client with its server name.
type Backend struct {
	Name   string
	Client client.MCPClient
}

// NewBackend creates a new backend connection based on the server config.
func NewBackend(ctx context.Context, name string, sc *config.ServerConfig) (*Backend, error) {
	var c client.MCPClient
	var err error

	switch sc.Transport() {
	case config.TransportStdio:
		c, err = newStdioClient(sc)
	case config.TransportSSE:
		c, err = newSSEClient(ctx, sc)
	case config.TransportStreamableHTTP:
		c, err = newStreamableHTTPClient(ctx, sc)
	default:
		return nil, fmt.Errorf("unknown transport for server %q", name)
	}
	if err != nil {
		return nil, fmt.Errorf("creating client for %q: %w", name, err)
	}

	return &Backend{Name: name, Client: c}, nil
}

// Initialize performs the MCP initialize handshake.
func (b *Backend) Initialize(ctx context.Context) (*mcp.InitializeResult, error) {
	req := mcp.InitializeRequest{}
	req.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	req.Params.ClientInfo = mcp.Implementation{
		Name:    "mcp-proxy",
		Version: "1.0.0",
	}
	return b.Client.Initialize(ctx, req)
}

// Close shuts down the backend client connection.
func (b *Backend) Close() error {
	return b.Client.Close()
}
