package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"git.wtr.app/watura/mcp-proxy/internal/backend"
	"git.wtr.app/watura/mcp-proxy/internal/config"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// ProxySession holds per-session backend connections.
type ProxySession struct {
	ID       string
	Backends map[string]*backend.Backend // serverName -> backend
	LastUsed time.Time
	mu       sync.Mutex
}

// Touch updates the last-used timestamp.
func (ps *ProxySession) Touch() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.LastUsed = time.Now()
}

// Close shuts down all backend connections for this session.
func (ps *ProxySession) Close() {
	for name, b := range ps.Backends {
		if err := b.Close(); err != nil {
			slog.Warn("error closing backend", "server", name, "session", ps.ID, "error", err)
		}
	}
}

// SessionManager manages per-agent sessions.
type SessionManager struct {
	cfg            *config.Config
	sessions       map[string]*ProxySession
	mu             sync.RWMutex
	sessionTimeout time.Duration
	shuttingDown   bool
	stopCh         chan struct{}
}

// NewSessionManager creates a new session manager.
func NewSessionManager(cfg *config.Config, sessionTimeout time.Duration) *SessionManager {
	sm := &SessionManager{
		cfg:            cfg,
		sessions:       make(map[string]*ProxySession),
		sessionTimeout: sessionTimeout,
		stopCh:         make(chan struct{}),
	}
	go sm.reapLoop()
	return sm
}

// CreateSession creates a new session with backend connections for all configured servers.
func (sm *SessionManager) CreateSession(ctx context.Context, sessionID string) (*ProxySession, error) {
	sm.mu.Lock()
	if sm.shuttingDown {
		sm.mu.Unlock()
		return nil, fmt.Errorf("server is shutting down")
	}
	sm.mu.Unlock()

	backends := make(map[string]*backend.Backend)
	for name, sc := range sm.cfg.MCPServers {
		b, err := backend.NewBackend(ctx, name, sc)
		if err != nil {
			slog.Error("failed to create backend", "server", name, "session", sessionID, "error", err)
			continue
		}
		if _, err := b.Initialize(ctx); err != nil {
			slog.Error("failed to initialize backend", "server", name, "session", sessionID, "error", err)
			b.Close()
			continue
		}
		backends[name] = b
		slog.Info("backend connected", "server", name, "session", sessionID)
	}

	if len(backends) == 0 {
		return nil, fmt.Errorf("no backends could be started for session %s", sessionID)
	}

	session := &ProxySession{
		ID:       sessionID,
		Backends: backends,
		LastUsed: time.Now(),
	}

	sm.mu.Lock()
	sm.sessions[sessionID] = session
	sm.mu.Unlock()

	slog.Info("session created", "session", sessionID, "backends", len(backends))
	return session, nil
}

// GetSession retrieves a session by ID.
func (sm *SessionManager) GetSession(sessionID string) (*ProxySession, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[sessionID]
	if ok {
		s.Touch()
	}
	return s, ok
}

// DestroySession closes and removes a session.
func (sm *SessionManager) DestroySession(sessionID string) {
	sm.mu.Lock()
	session, ok := sm.sessions[sessionID]
	if ok {
		delete(sm.sessions, sessionID)
	}
	sm.mu.Unlock()

	if ok {
		session.Close()
		slog.Info("session destroyed", "session", sessionID)
	}
}

// Shutdown stops accepting new sessions and closes all existing ones.
func (sm *SessionManager) Shutdown() {
	sm.mu.Lock()
	sm.shuttingDown = true
	close(sm.stopCh)
	sessions := make(map[string]*ProxySession, len(sm.sessions))
	for k, v := range sm.sessions {
		sessions[k] = v
	}
	sm.sessions = make(map[string]*ProxySession)
	sm.mu.Unlock()

	for _, s := range sessions {
		s.Close()
	}
}

func (sm *SessionManager) reapLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-sm.stopCh:
			return
		case <-ticker.C:
			sm.reapExpired()
		}
	}
}

func (sm *SessionManager) reapExpired() {
	sm.mu.Lock()
	var expired []string
	now := time.Now()
	for id, s := range sm.sessions {
		s.mu.Lock()
		if now.Sub(s.LastUsed) > sm.sessionTimeout {
			expired = append(expired, id)
		}
		s.mu.Unlock()
	}
	toClose := make([]*ProxySession, 0, len(expired))
	for _, id := range expired {
		toClose = append(toClose, sm.sessions[id])
		delete(sm.sessions, id)
	}
	sm.mu.Unlock()

	for _, s := range toClose {
		slog.Info("session expired", "session", s.ID)
		s.Close()
	}
}

// ListBackendTools lists tools from a specific backend in a session.
func ListBackendTools(ctx context.Context, c client.MCPClient) ([]mcp.Tool, error) {
	result, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// ListBackendResources lists resources from a specific backend in a session.
func ListBackendResources(ctx context.Context, c client.MCPClient) ([]mcp.Resource, error) {
	result, err := c.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		return nil, err
	}
	return result.Resources, nil
}

// ListBackendPrompts lists prompts from a specific backend in a session.
func ListBackendPrompts(ctx context.Context, c client.MCPClient) ([]mcp.Prompt, error) {
	result, err := c.ListPrompts(ctx, mcp.ListPromptsRequest{})
	if err != nil {
		return nil, err
	}
	return result.Prompts, nil
}
