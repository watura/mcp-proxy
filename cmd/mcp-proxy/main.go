// Command mcp-proxy aggregates multiple backend MCP servers defined in an
// mcp.json config file and exposes them through a single endpoint.
//
// When invoked with only --config (no --port/--addr and no addr/port in the
// config), it serves over stdio so an MCP client can launch the binary
// directly. When --port/--addr is given explicitly, or "addr"/"port" is set in
// mcp.json, it serves over HTTP (SSE + Streamable HTTP). Explicit flags take
// precedence over the config values.
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

	// Resolve listen address and HTTP mode. Precedence: explicit --addr/--port
	// flags win, then mcp.json's addr/port, otherwise stdio mode.
	listenAddr, listenPort := *addr, *port
	httpMode := flagPassed("addr") || flagPassed("port")
	if !flagPassed("addr") && cfg.Addr != "" {
		listenAddr = cfg.Addr
		httpMode = true
	}
	if !flagPassed("port") && cfg.Port != 0 {
		listenPort = cfg.Port
		httpMode = true
	}

	ps, err := proxy.NewProxyServer(&proxy.ProxyServerConfig{
		Config: cfg,
		Addr:   net.JoinHostPort(listenAddr, strconv.Itoa(listenPort)),
	})
	if err != nil {
		slog.Error("failed to create proxy server", "error", err)
		os.Exit(1)
	}

	// stdio mode unless an HTTP address was configured (via flag or mcp.json).
	if !httpMode {
		if err := ps.ServeStdio(); err != nil {
			slog.Error("stdio server error", "error", err)
			os.Exit(1)
		}
		return
	}

	runHTTP(ps)
}

// flagPassed reports whether the named flag was explicitly provided on the CLI.
func flagPassed(name string) bool {
	passed := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			passed = true
		}
	})
	return passed
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
