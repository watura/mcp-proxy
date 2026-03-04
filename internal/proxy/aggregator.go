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

// Aggregate merges capabilities from multiple backends, resolving tool/prompt name conflicts.
func Aggregate(servers []ServerCapabilities) *AggregatedCapabilities {
	agg := &AggregatedCapabilities{
		Tools:     make(map[string]ToolMapping),
		Resources: make(map[string]ResourceMapping),
		Prompts:   make(map[string]PromptMapping),
	}

	agg.Tools = resolveToolConflicts(servers)
	agg.Prompts = resolvePromptConflicts(servers)

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

func resolveToolConflicts(servers []ServerCapabilities) map[string]ToolMapping {
	// Step 1: build map[toolName][]serverName
	nameToServers := make(map[string][]string)
	for _, srv := range servers {
		for _, tool := range srv.Tools {
			nameToServers[tool.Name] = append(nameToServers[tool.Name], srv.Name)
		}
	}

	// Step 2: determine conflicting names
	conflicting := make(map[string]bool)
	for name, srvs := range nameToServers {
		if len(srvs) > 1 {
			conflicting[name] = true
		}
	}

	// Step 3: build result
	result := make(map[string]ToolMapping)
	for _, srv := range servers {
		for _, tool := range srv.Tools {
			proxyName := tool.Name
			if conflicting[tool.Name] {
				proxyName = srv.Name + "__" + tool.Name
			}
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

func resolvePromptConflicts(servers []ServerCapabilities) map[string]PromptMapping {
	nameToServers := make(map[string][]string)
	for _, srv := range servers {
		for _, prompt := range srv.Prompts {
			nameToServers[prompt.Name] = append(nameToServers[prompt.Name], srv.Name)
		}
	}

	conflicting := make(map[string]bool)
	for name, srvs := range nameToServers {
		if len(srvs) > 1 {
			conflicting[name] = true
		}
	}

	result := make(map[string]PromptMapping)
	for _, srv := range servers {
		for _, prompt := range srv.Prompts {
			proxyName := prompt.Name
			if conflicting[prompt.Name] {
				proxyName = srv.Name + "__" + prompt.Name
			}
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
