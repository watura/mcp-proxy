// Command mcp-proxy aggregates multiple backend MCP servers defined in an
// mcp.json config file and exposes them through a single endpoint.
//
// When invoked with only --config (no --port/--addr), it serves over stdio
// so an MCP client can launch the binary directly. When --port or --addr is
// given explicitly, it serves over HTTP (SSE + Streamable HTTP).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"git.wtr.app/watura/mcp-proxy/internal/config"
	"git.wtr.app/watura/mcp-proxy/internal/proxy"
)

func main() {
	configPath := flag.String("config", "mcp.json", "path to mcp.json config file")
	port := flag.Int("port", 8080, "listen port (HTTP mode)")
	addr := flag.String("addr", "127.0.0.1", "listen address (HTTP mode)")
	logLevel := flag.String("log-level", "info", "log level (debug/info/warn/error)")
	flag.Parse()

	// Logs must go to stderr: in stdio mode stdout carries the JSON-RPC stream.
	level, err := parseLogLevel(*logLevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		flag.Usage()
		os.Exit(2)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ps, err := proxy.NewProxyServer(&proxy.ProxyServerConfig{
		Config: cfg,
		Addr:   net.JoinHostPort(*addr, strconv.Itoa(*port)),
	})
	if err != nil {
		slog.Error("failed to create proxy server", "error", err)
		os.Exit(1)
	}

	// stdio mode unless an HTTP flag (--port/--addr) was explicitly provided.
	if !httpFlagsSet() {
		if err := ps.ServeStdio(); err != nil {
			slog.Error("stdio server error", "error", err)
			os.Exit(1)
		}
		return
	}

	runHTTP(ps)
}

// httpFlagsSet reports whether --port or --addr was explicitly passed.
func httpFlagsSet() bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "port" || f.Name == "addr" {
			set = true
		}
	})
	return set
}

// runHTTP starts the HTTP server and shuts it down gracefully on SIGINT/SIGTERM.
func runHTTP(ps *proxy.ProxyServer) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		if err := ps.Start(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		slog.Error("http server error", "error", err)
		os.Exit(1)
	case <-ctx.Done():
		stop() // restore default signal handling so a second Ctrl-C force-quits
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := ps.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown error", "error", err)
			os.Exit(1)
		}
	}
}

func parseLogLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("error: invalid --log-level %q (want debug/info/warn/error)", s)
	}
}
