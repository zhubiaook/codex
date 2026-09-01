package codex_test

import (
	stdjson "encoding/json"
	"reflect"
	"testing"

	"github.com/zhubiaook/codex/sdk/go"
)

func TestThreadDecodesIntegrationAndPlanningItems(t *testing.T) {
	client := newActivityClient(t, "integration-items")
	var activity []codex.ThreadItem
	for event, err := range client.StartThread(codex.ThreadOptions{}).RunStreamed(
		t.Context(), "activity", codex.TurnOptions{},
	) {
		if err != nil {
			t.Fatalf("RunStreamed() error = %v", err)
		}
		switch event := event.(type) {
		case *codex.ItemStartedEvent:
			activity = append(activity, event.Item)
		case *codex.ItemUpdatedEvent:
			activity = append(activity, event.Item)
		case *codex.ItemCompletedEvent:
			activity = append(activity, event.Item)
		}
	}
	want := []codex.ThreadItem{
		&codex.MCPToolCallItem{
			ID: "mcp-start", Server: "files", Tool: "read", Arguments: stdjson.RawMessage(`{"path":"README.md"}`), Status: codex.MCPToolCallInProgress,
		},
		&codex.MCPToolCallItem{
			ID: "mcp-start", Server: "files", Tool: "read", Arguments: stdjson.RawMessage(`{"path":"README.md"}`),
			Result: &codex.MCPToolCallResult{Content: []stdjson.RawMessage{}, StructuredContent: stdjson.RawMessage(`null`)},
			Status: codex.MCPToolCallInProgress,
		},
		&codex.MCPToolCallItem{
			ID: "mcp-success", Server: "db", Tool: "query", Arguments: stdjson.RawMessage(`{"sql":"select 1"}`),
			Result: &codex.MCPToolCallResult{
				Content:           []stdjson.RawMessage{stdjson.RawMessage(`{"type":"text","text":"1"}`)},
				Meta:              stdjson.RawMessage(`{"trace":"abc"}`),
				StructuredContent: stdjson.RawMessage(`{"rows":[1]}`),
			},
			Status: codex.MCPToolCallCompleted,
		},
		&codex.MCPToolCallItem{
			ID: "mcp-failure", Server: "db", Tool: "query", Arguments: stdjson.RawMessage(`null`),
			Error: &codex.MCPToolCallError{Message: "permission denied"}, Status: codex.MCPToolCallFailed,
		},
		&codex.WebSearchItem{ID: "search-1", Query: "Go 1.27 release notes"},
		&codex.TodoListItem{ID: "todo-1", Items: []codex.TodoItem{
			{Text: "Inspect", Completed: true},
			{Text: "Implement", Completed: false},
		}},
		&codex.ErrorItem{ID: "error-1", Message: "non-fatal warning"},
	}
	if !reflect.DeepEqual(activity, want) {
		t.Errorf("integration activity = %#v, want %#v", activity, want)
	}

	turn, err := newActivityClient(t, "integration-items").StartThread(codex.ThreadOptions{}).Run(
		t.Context(), "activity", codex.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !reflect.DeepEqual(turn.Items, want[2:]) {
		t.Errorf("Turn.Items = %#v, want %#v", turn.Items, want[2:])
	}
}
