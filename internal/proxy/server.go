package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"git.wtr.app/watura/mcp-proxy/internal/config"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ProxyServer is the main proxy server that aggregates multiple MCP backends.
type ProxyServer struct {
	sharedSession *ProxySession
	dispatcher    *Dispatcher
	mcpServer     *server.MCPServer
	httpServer    *http.Server
	addr          string
}

// ProxyServerConfig holds configuration for creating a ProxyServer.
type ProxyServerConfig struct {
	Config *config.Config
	Addr   string
}

// NewProxyServer creates and configures a new proxy server.
func NewProxyServer(psc *ProxyServerConfig) (*ProxyServer, error) {
	// Create shared session: connect backends and discover capabilities in one step
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	session, err := CreateSharedSession(ctx, psc.Config)
	if err != nil {
		return nil, fmt.Errorf("create shared session: %w", err)
	}

	caps := session.Caps
	ps := &ProxyServer{
		sharedSession: session,
		dispatcher:    NewDispatcher(caps),
		addr:          psc.Addr,
	}

	// Create MCP server
	var opts []server.ServerOption
	if len(caps.Tools) > 0 {
		opts = append(opts, server.WithToolCapabilities(true))
	}
	if len(caps.Resources) > 0 {
		opts = append(opts, server.WithResourceCapabilities(true, true))
	}
	if len(caps.Prompts) > 0 {
		opts = append(opts, server.WithPromptCapabilities(true))
	}

	ps.mcpServer = server.NewMCPServer("mcp-proxy", "1.0.0", opts...)

	// Register tools globally (shared across all clients)
	for proxyName, tm := range caps.Tools {
		ps.mcpServer.AddTool(tm.Tool, ps.makeToolHandler(proxyName))
	}

	// Register resources globally
	for _, rm := range caps.Resources {
		ps.mcpServer.AddResource(rm.Resource, ps.makeResourceHandler(rm.Resource.URI))
	}

	// Register prompts globally
	for _, pm := range caps.Prompts {
		ps.mcpServer.AddPrompt(pm.Prompt, ps.makePromptHandler(pm.Prompt.Name))
	}

	return ps, nil
}

func (ps *ProxyServer) makeToolHandler(proxyName string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return ps.dispatcher.CallTool(ctx, ps.sharedSession, proxyName, req.GetArguments())
	}
}

func (ps *ProxyServer) makeResourceHandler(uri string) server.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		result, err := ps.dispatcher.ReadResource(ctx, ps.sharedSession, uri)
		if err != nil {
			return nil, err
		}
		return result.Contents, nil
	}
}

func (ps *ProxyServer) makePromptHandler(proxyName string) server.PromptHandlerFunc {
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return ps.dispatcher.GetPrompt(ctx, ps.sharedSession, proxyName, req.Params.Arguments)
	}
}

// Start starts the HTTP server serving both SSE and Streamable HTTP transports.
func (ps *ProxyServer) Start() error {
	sseServer := server.NewSSEServer(ps.mcpServer,
		server.WithBaseURL(fmt.Sprintf("http://%s", ps.addr)),
		server.WithSSEEndpoint("/sse"),
		server.WithMessageEndpoint("/message"),
	)

	streamServer := server.NewStreamableHTTPServer(ps.mcpServer,
		server.WithEndpointPath("/mcp"),
	)

	mux := http.NewServeMux()
	mux.Handle("/sse", sseServer.SSEHandler())
	mux.Handle("/message", sseServer.MessageHandler())
	mux.Handle("/mcp", streamServer)

	ps.httpServer = &http.Server{
		Addr:    ps.addr,
		Handler: mux,
	}

	caps := ps.sharedSession.Caps
	slog.Info("starting proxy server", "addr", ps.addr,
		"tools", len(caps.Tools),
		"resources", len(caps.Resources),
		"prompts", len(caps.Prompts))

	return ps.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the proxy server.
func (ps *ProxyServer) Shutdown(ctx context.Context) error {
	slog.Info("shutting down proxy server")
	err := ps.httpServer.Shutdown(ctx)
	ps.sharedSession.Close()
	return err
}
