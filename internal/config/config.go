package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Transport represents the connection type for a backend MCP server.
type Transport int

const (
	TransportStdio Transport = iota
	TransportSSE
	TransportStreamableHTTP
)

// ServerConfig represents a single MCP server definition from mcp.json.
type ServerConfig struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
}

// Transport returns the transport type for this server config.
func (sc *ServerConfig) Transport() Transport {
	if sc.Command != "" {
		return TransportStdio
	}
	if sc.URL != "" {
		if strings.HasSuffix(sc.URL, "/sse") {
			return TransportSSE
		}
		return TransportStreamableHTTP
	}
	return TransportStdio
}

// EnvList returns the environment variables as a slice of "KEY=VALUE" strings,
// merged with the current process environment.
func (sc *ServerConfig) EnvList() []string {
	env := os.Environ()
	for k, v := range sc.Env {
		env = append(env, k+"="+v)
	}
	return env
}

// Config represents the top-level mcp.json configuration.
type Config struct {
	MCPServers map[string]*ServerConfig `json:"mcpServers"`
}

// Load reads and parses an mcp.json file from the given path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	if cfg.MCPServers == nil || len(cfg.MCPServers) == 0 {
		return nil, fmt.Errorf("no mcpServers defined in config")
	}

	for name, sc := range cfg.MCPServers {
		if sc.Command == "" && sc.URL == "" {
			return nil, fmt.Errorf("server %q: must specify either 'command' or 'url'", name)
		}
	}

	return &cfg, nil
}
