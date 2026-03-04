package proxy

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestAggregateNoConflicts(t *testing.T) {
	servers := []ServerCapabilities{
		{
			Name:  "server-a",
			Tools: []mcp.Tool{newTool("tool1"), newTool("tool2")},
		},
		{
			Name:  "server-b",
			Tools: []mcp.Tool{newTool("tool3")},
		},
	}

	agg := Aggregate(servers)

	if len(agg.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(agg.Tools))
	}
	for _, name := range []string{"tool1", "tool2", "tool3"} {
		if _, ok := agg.Tools[name]; !ok {
			t.Errorf("expected tool %q in result", name)
		}
	}
}

func TestAggregateWithConflicts(t *testing.T) {
	servers := []ServerCapabilities{
		{
			Name:  "server-a",
			Tools: []mcp.Tool{newTool("read"), newTool("unique-a")},
		},
		{
			Name:  "server-b",
			Tools: []mcp.Tool{newTool("read"), newTool("unique-b")},
		},
	}

	agg := Aggregate(servers)

	if len(agg.Tools) != 4 {
		t.Fatalf("expected 4 tools, got %d", len(agg.Tools))
	}

	if _, ok := agg.Tools["server-a__read"]; !ok {
		t.Error("expected 'server-a__read' in result")
	}
	if _, ok := agg.Tools["server-b__read"]; !ok {
		t.Error("expected 'server-b__read' in result")
	}
	if _, ok := agg.Tools["unique-a"]; !ok {
		t.Error("expected 'unique-a' in result")
	}
	if _, ok := agg.Tools["unique-b"]; !ok {
		t.Error("expected 'unique-b' in result")
	}

	// Verify original name is preserved
	m := agg.Tools["server-a__read"]
	if m.OriginalName != "read" {
		t.Errorf("expected original name 'read', got %q", m.OriginalName)
	}
}

func TestAggregatePromptConflicts(t *testing.T) {
	servers := []ServerCapabilities{
		{
			Name:    "server-a",
			Prompts: []mcp.Prompt{{Name: "greeting"}},
		},
		{
			Name:    "server-b",
			Prompts: []mcp.Prompt{{Name: "greeting"}},
		},
	}

	agg := Aggregate(servers)

	if len(agg.Prompts) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(agg.Prompts))
	}
	if _, ok := agg.Prompts["server-a__greeting"]; !ok {
		t.Error("expected 'server-a__greeting'")
	}
	if _, ok := agg.Prompts["server-b__greeting"]; !ok {
		t.Error("expected 'server-b__greeting'")
	}
}

func TestAggregateResources(t *testing.T) {
	servers := []ServerCapabilities{
		{
			Name:      "server-a",
			Resources: []mcp.Resource{{URI: "file:///a", Name: "a"}},
		},
		{
			Name:      "server-b",
			Resources: []mcp.Resource{{URI: "file:///b", Name: "b"}},
		},
	}

	agg := Aggregate(servers)

	if len(agg.Resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(agg.Resources))
	}
}

func newTool(name string) mcp.Tool {
	return mcp.Tool{Name: name}
}
