package backend

import (
	"context"

	"git.wtr.app/watura/mcp-proxy/internal/config"
	"github.com/mark3labs/mcp-go/client"
)

func newSSEClient(ctx context.Context, sc *config.ServerConfig) (client.MCPClient, error) {
	c, err := client.NewSSEMCPClient(sc.URL)
	if err != nil {
		return nil, err
	}
	if err := c.Start(ctx); err != nil {
		return nil, err
	}
	return c, nil
}
