package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Run("valid stdio config", func(t *testing.T) {
		content := `{
			"mcpServers": {
				"filesystem": {
					"command": "npx",
					"args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
					"env": {"FOO": "bar"}
				}
			}
		}`
		path := writeTemp(t, content)

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sc := cfg.MCPServers["filesystem"]
		if sc == nil {
			t.Fatal("expected 'filesystem' server config")
		}
		if sc.Command != "npx" {
			t.Errorf("expected command 'npx', got %q", sc.Command)
		}
		if sc.Transport() != TransportStdio {
			t.Errorf("expected TransportStdio, got %v", sc.Transport())
		}
		if len(sc.Args) != 3 {
			t.Errorf("expected 3 args, got %d", len(sc.Args))
		}
	})

	t.Run("valid sse config", func(t *testing.T) {
		content := `{
			"mcpServers": {
				"remote": {
					"url": "http://localhost:9090/sse"
				}
			}
		}`
		path := writeTemp(t, content)

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sc := cfg.MCPServers["remote"]
		if sc.Transport() != TransportSSE {
			t.Errorf("expected TransportSSE, got %v", sc.Transport())
		}
	})

	t.Run("valid streamable http config", func(t *testing.T) {
		content := `{
			"mcpServers": {
				"remote": {
					"url": "http://localhost:9090/mcp"
				}
			}
		}`
		path := writeTemp(t, content)

		cfg, err := Load(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sc := cfg.MCPServers["remote"]
		if sc.Transport() != TransportStreamableHTTP {
			t.Errorf("expected TransportStreamableHTTP, got %v", sc.Transport())
		}
	})

	t.Run("no servers", func(t *testing.T) {
		content := `{"mcpServers": {}}`
		path := writeTemp(t, content)

		_, err := Load(path)
		if err == nil {
			t.Fatal("expected error for empty mcpServers")
		}
	})

	t.Run("missing command and url", func(t *testing.T) {
		content := `{"mcpServers": {"bad": {}}}`
		path := writeTemp(t, content)

		_, err := Load(path)
		if err == nil {
			t.Fatal("expected error for missing command/url")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := Load("/nonexistent/path.json")
		if err == nil {
			t.Fatal("expected error for missing file")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		path := writeTemp(t, "not json")

		_, err := Load(path)
		if err == nil {
			t.Fatal("expected error for invalid json")
		}
	})
}

func TestServerConfigEnvList(t *testing.T) {
	sc := &ServerConfig{
		Env: map[string]string{"MY_VAR": "hello"},
	}
	envList := sc.EnvList()
	found := false
	for _, e := range envList {
		if e == "MY_VAR=hello" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected MY_VAR=hello in env list")
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}
