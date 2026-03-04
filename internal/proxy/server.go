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
	cfg            *config.Config
	sessionMgr     *SessionManager
	dispatcher     *Dispatcher
	mcpServer      *server.MCPServer
	httpServer     *http.Server
	caps           *AggregatedCapabilities
	addr           string
	sessionTimeout time.Duration
}

// ProxyServerConfig holds configuration for creating a ProxyServer.
type ProxyServerConfig struct {
	Config         *config.Config
	Addr           string
	SessionTimeout time.Duration
}

// NewProxyServer creates and configures a new proxy server.
func NewProxyServer(psc *ProxyServerConfig) (*ProxyServer, error) {
	ps := &ProxyServer{
		cfg:            psc.Config,
		addr:           psc.Addr,
		sessionTimeout: psc.SessionTimeout,
	}

	// Bootstrap: create temporary connections to discover capabilities
	caps, err := ps.bootstrap()
	if err != nil {
		return nil, fmt.Errorf("bootstrap: %w", err)
	}
	ps.caps = caps
	ps.dispatcher = NewDispatcher(caps)
	ps.sessionMgr = NewSessionManager(psc.Config, psc.SessionTimeout)

	// Create MCP server with hooks
	hooks := &server.Hooks{}
	hooks.AddOnRegisterSession(ps.onRegisterSession)
	hooks.AddOnUnregisterSession(ps.onUnregisterSession)

	opts := []server.ServerOption{
		server.WithHooks(hooks),
	}
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

	// Register prompts globally (since AddSessionPrompt doesn't exist)
	for _, pm := range caps.Prompts {
		proxyName := pm.Prompt.Name
		ps.mcpServer.AddPrompt(pm.Prompt, ps.makePromptHandler(proxyName))
	}

	return ps, nil
}

// bootstrap creates temporary connections to each backend to discover capabilities.
func (ps *ProxyServer) bootstrap() (*AggregatedCapabilities, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var servers []ServerCapabilities
	for name, sc := range ps.cfg.MCPServers {
		slog.Info("bootstrapping backend", "server", name)

		caps, err := ps.bootstrapServer(ctx, name, sc)
		if err != nil {
			slog.Error("bootstrap failed, skipping server", "server", name, "error", err)
			continue
		}
		servers = append(servers, *caps)
	}

	if len(servers) == 0 {
		return nil, fmt.Errorf("no backends available after bootstrap")
	}

	return Aggregate(servers), nil
}

func (ps *ProxyServer) bootstrapServer(ctx context.Context, name string, sc *config.ServerConfig) (*ServerCapabilities, error) {
	var b interface {
		Initialize(context.Context) (*mcp.InitializeResult, error)
		Close() error
	}

	// Import backend package for creating bootstrap connections
	bk, err := newBootstrapBackend(ctx, name, sc)
	if err != nil {
		return nil, err
	}
	b = bk
	defer b.Close()

	initResult, err := b.Initialize(ctx)
	if err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}

	caps := &ServerCapabilities{Name: name}

	if initResult.Capabilities.Tools != nil {
		tools, err := ListBackendTools(ctx, bk.Client)
		if err != nil {
			slog.Warn("failed to list tools", "server", name, "error", err)
		} else {
			caps.Tools = tools
		}
	}

	if initResult.Capabilities.Resources != nil {
		resources, err := ListBackendResources(ctx, bk.Client)
		if err != nil {
			slog.Warn("failed to list resources", "server", name, "error", err)
		} else {
			caps.Resources = resources
		}
	}

	if initResult.Capabilities.Prompts != nil {
		prompts, err := ListBackendPrompts(ctx, bk.Client)
		if err != nil {
			slog.Warn("failed to list prompts", "server", name, "error", err)
		} else {
			caps.Prompts = prompts
		}
	}

	slog.Info("bootstrap complete", "server", name,
		"tools", len(caps.Tools),
		"resources", len(caps.Resources),
		"prompts", len(caps.Prompts))

	return caps, nil
}

func (ps *ProxyServer) onRegisterSession(ctx context.Context, session server.ClientSession) {
	sessionID := session.SessionID()
	slog.Info("registering session", "session", sessionID)

	proxySession, err := ps.sessionMgr.CreateSession(ctx, sessionID)
	if err != nil {
		slog.Error("failed to create session", "session", sessionID, "error", err)
		return
	}

	// Register session-specific tools
	for proxyName, tm := range ps.caps.Tools {
		tool := tm.Tool
		handler := ps.makeToolHandler(proxyName)
		if err := ps.mcpServer.AddSessionTool(sessionID, tool, handler); err != nil {
			slog.Error("failed to add session tool", "session", sessionID, "tool", proxyName, "error", err)
		}
	}

	// Register session-specific resources
	for _, rm := range ps.caps.Resources {
		handler := ps.makeResourceHandler(rm.Resource.URI)
		if err := ps.mcpServer.AddSessionResource(sessionID, rm.Resource, handler); err != nil {
			slog.Error("failed to add session resource", "session", sessionID, "resource", rm.Resource.URI, "error", err)
		}
	}

	_ = proxySession // already stored in session manager
}

func (ps *ProxyServer) onUnregisterSession(ctx context.Context, session server.ClientSession) {
	sessionID := session.SessionID()
	slog.Info("unregistering session", "session", sessionID)
	ps.sessionMgr.DestroySession(sessionID)
}

func (ps *ProxyServer) makeToolHandler(proxyName string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		session := server.ClientSessionFromContext(ctx)
		if session == nil {
			return mcp.NewToolResultError("no session"), nil
		}

		proxySession, ok := ps.sessionMgr.GetSession(session.SessionID())
		if !ok {
			return mcp.NewToolResultError("session not found"), nil
		}

		return ps.dispatcher.CallTool(ctx, proxySession, proxyName, req.GetArguments())
	}
}

func (ps *ProxyServer) makeResourceHandler(uri string) server.ResourceHandlerFunc {
	return func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		session := server.ClientSessionFromContext(ctx)
		if session == nil {
			return nil, fmt.Errorf("no session")
		}

		proxySession, ok := ps.sessionMgr.GetSession(session.SessionID())
		if !ok {
			return nil, fmt.Errorf("session not found")
		}

		result, err := ps.dispatcher.ReadResource(ctx, proxySession, uri)
		if err != nil {
			return nil, err
		}

		return result.Contents, nil
	}
}

func (ps *ProxyServer) makePromptHandler(proxyName string) server.PromptHandlerFunc {
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		session := server.ClientSessionFromContext(ctx)
		if session == nil {
			return nil, fmt.Errorf("no session")
		}

		proxySession, ok := ps.sessionMgr.GetSession(session.SessionID())
		if !ok {
			return nil, fmt.Errorf("session not found")
		}

		return ps.dispatcher.GetPrompt(ctx, proxySession, proxyName, req.Params.Arguments)
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

	slog.Info("starting proxy server", "addr", ps.addr,
		"tools", len(ps.caps.Tools),
		"resources", len(ps.caps.Resources),
		"prompts", len(ps.caps.Prompts))

	return ps.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the proxy server.
func (ps *ProxyServer) Shutdown(ctx context.Context) error {
	slog.Info("shutting down proxy server")
	ps.sessionMgr.Shutdown()
	return ps.httpServer.Shutdown(ctx)
}
