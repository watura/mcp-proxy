package backend

import (
	"git.wtr.app/watura/mcp-proxy/internal/config"
	"github.com/mark3labs/mcp-go/client"
)

func newStdioClient(sc *config.ServerConfig) (client.MCPClient, error) {
	return client.NewStdioMCPClient(sc.Command, sc.EnvList(), sc.Args...)
}
