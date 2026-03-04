package proxy

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// Dispatcher routes proxy requests to the correct backend.
type Dispatcher struct {
	caps *AggregatedCapabilities
}

// NewDispatcher creates a new dispatcher with the given aggregated capabilities.
func NewDispatcher(caps *AggregatedCapabilities) *Dispatcher {
	return &Dispatcher{caps: caps}
}

// CallTool dispatches a tool call to the appropriate backend.
func (d *Dispatcher) CallTool(ctx context.Context, session *ProxySession, proxyName string, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	session.Touch()

	mapping, ok := d.caps.Tools[proxyName]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", proxyName)
	}

	b, ok := session.Backends[mapping.ServerName]
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf("backend %q not available in this session", mapping.ServerName)), nil
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = mapping.OriginalName
	req.Params.Arguments = arguments

	result, err := b.Client.CallTool(ctx, req)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("error calling tool %q on %q: %v", mapping.OriginalName, mapping.ServerName, err)), nil
	}

	return result, nil
}

// ReadResource dispatches a resource read to the appropriate backend.
func (d *Dispatcher) ReadResource(ctx context.Context, session *ProxySession, uri string) (*mcp.ReadResourceResult, error) {
	session.Touch()

	mapping, ok := d.caps.Resources[uri]
	if !ok {
		return nil, fmt.Errorf("unknown resource: %s", uri)
	}

	b, ok := session.Backends[mapping.ServerName]
	if !ok {
		return nil, fmt.Errorf("backend %q not available in this session", mapping.ServerName)
	}

	req := mcp.ReadResourceRequest{}
	req.Params.URI = uri

	return b.Client.ReadResource(ctx, req)
}

// GetPrompt dispatches a prompt request to the appropriate backend.
func (d *Dispatcher) GetPrompt(ctx context.Context, session *ProxySession, proxyName string, arguments map[string]string) (*mcp.GetPromptResult, error) {
	session.Touch()

	mapping, ok := d.caps.Prompts[proxyName]
	if !ok {
		return nil, fmt.Errorf("unknown prompt: %s", proxyName)
	}

	b, ok := session.Backends[mapping.ServerName]
	if !ok {
		return nil, fmt.Errorf("backend %q not available in this session", mapping.ServerName)
	}

	req := mcp.GetPromptRequest{}
	req.Params.Name = mapping.OriginalName
	req.Params.Arguments = arguments

	return b.Client.GetPrompt(ctx, req)
}
