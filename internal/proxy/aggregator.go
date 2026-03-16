package proxy

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// ToolMapping maps a proxy-visible tool name to its backend server name and original tool.
type ToolMapping struct {
	ServerName   string
	OriginalName string
	Tool         mcp.Tool
}

// ResourceMapping maps a resource URI to its backend server name and original resource.
type ResourceMapping struct {
	ServerName string
	Resource   mcp.Resource
}

// PromptMapping maps a prompt name to its backend server name and original prompt.
type PromptMapping struct {
	ServerName   string
	OriginalName string
	Prompt       mcp.Prompt
}

// AggregatedCapabilities holds the merged tools/resources/prompts from all backends.
type AggregatedCapabilities struct {
	Tools     map[string]ToolMapping     // proxyName -> mapping
	Resources map[string]ResourceMapping // URI -> mapping
	Prompts   map[string]PromptMapping   // proxyName -> mapping
}

// ServerCapabilities holds the capabilities for a single backend server.
type ServerCapabilities struct {
	Name      string
	Tools     []mcp.Tool
	Resources []mcp.Resource
	Prompts   []mcp.Prompt
}

// Aggregate merges capabilities from multiple backends, prefixing tool/prompt names with the server name.
func Aggregate(servers []ServerCapabilities) *AggregatedCapabilities {
	agg := &AggregatedCapabilities{
		Tools:     make(map[string]ToolMapping),
		Resources: make(map[string]ResourceMapping),
		Prompts:   make(map[string]PromptMapping),
	}

	agg.Tools = prefixTools(servers)
	agg.Prompts = prefixPrompts(servers)

	// Resources are keyed by URI, no conflict resolution needed
	for _, srv := range servers {
		for _, r := range srv.Resources {
			agg.Resources[r.URI] = ResourceMapping{
				ServerName: srv.Name,
				Resource:   r,
			}
		}
	}

	return agg
}

func prefixTools(servers []ServerCapabilities) map[string]ToolMapping {
	result := make(map[string]ToolMapping)
	for _, srv := range servers {
		for _, tool := range srv.Tools {
			proxyName := srv.Name + "__" + tool.Name
			prefixed := tool
			prefixed.Name = proxyName
			result[proxyName] = ToolMapping{
				ServerName:   srv.Name,
				OriginalName: tool.Name,
				Tool:         prefixed,
			}
		}
	}

	return result
}

func prefixPrompts(servers []ServerCapabilities) map[string]PromptMapping {
	result := make(map[string]PromptMapping)
	for _, srv := range servers {
		for _, prompt := range srv.Prompts {
			proxyName := srv.Name + "__" + prompt.Name
			prefixed := prompt
			prefixed.Name = proxyName
			result[proxyName] = PromptMapping{
				ServerName:   srv.Name,
				OriginalName: prompt.Name,
				Prompt:       prefixed,
			}
		}
	}

	return result
}
