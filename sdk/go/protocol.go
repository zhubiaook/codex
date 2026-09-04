package codex

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"slices"
)

type eventHeader struct {
	Type EventType `json:"type"`
}

type itemEventWire struct {
	Item jsontext.Value `json:"item"`
}

type itemHeader struct {
	ID   string   `json:"id"`
	Type ItemType `json:"type"`
}

func decodeThreadEvent(data []byte) (ThreadEvent, error) {
	var header eventHeader
	if err := jsonv2.Unmarshal(data, &header); err != nil {
		return nil, err
	}
	if header.Type == "" {
		return nil, protocolPayloadError("Thread Event has no type discriminator")
	}
	switch header.Type {
	case EventThreadStarted:
		var event ThreadStartedEvent
		if err := jsonv2.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		if event.ThreadID == "" {
			return nil, protocolPayloadError("thread.started has an empty thread_id")
		}
		return &event, nil
	case EventTurnStarted:
		return &TurnStartedEvent{}, nil
	case EventTurnCompleted:
		var event TurnCompletedEvent
		if err := jsonv2.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		members, err := requiredJSONMembers(data, "usage")
		if err != nil {
			return nil, err
		}
		if _, err := requiredJSONMembers(members["usage"], "input_tokens", "cached_input_tokens", "output_tokens", "reasoning_output_tokens"); err != nil {
			return nil, fmt.Errorf("turn.completed usage: %w", err)
		}
		return &event, nil
	case EventTurnFailed:
		var event TurnFailedEvent
		if err := jsonv2.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		if event.Error.Message == "" {
			return nil, protocolPayloadError("turn.failed has an empty error message")
		}
		return &event, nil
	case EventItemStarted, EventItemUpdated, EventItemCompleted:
		var wire itemEventWire
		if err := jsonv2.Unmarshal(data, &wire); err != nil {
			return nil, err
		}
		if wire.Item == nil {
			return nil, protocolPayloadError(fmt.Sprintf("%s has no item", header.Type))
		}
		item, err := decodeThreadItem(wire.Item)
		if err != nil {
			return nil, err
		}
		switch header.Type {
		case EventItemStarted:
			return &ItemStartedEvent{Item: item}, nil
		case EventItemUpdated:
			return &ItemUpdatedEvent{Item: item}, nil
		case EventItemCompleted:
			return &ItemCompletedEvent{Item: item}, nil
		}
	case EventError:
		var event ThreadErrorEvent
		if err := jsonv2.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		if event.Message == "" {
			return nil, protocolPayloadError("error event has an empty message")
		}
		return &event, nil
	default:
		return &UnknownEvent{UnknownType: header.Type, Raw: json.RawMessage(bytes.Clone(data))}, nil
	}
	return nil, protocolPayloadError(fmt.Sprintf("unsupported Thread Event %q", header.Type))
}

func decodeThreadItem(data []byte) (ThreadItem, error) {
	var header itemHeader
	if err := jsonv2.Unmarshal(data, &header); err != nil {
		return nil, err
	}
	if header.Type == "" {
		return nil, protocolPayloadError("Thread Item has no type discriminator")
	}
	if header.ID == "" {
		return nil, protocolPayloadError(fmt.Sprintf("%s Thread Item has an empty id", header.Type))
	}
	switch header.Type {
	case ItemAgentMessage:
		var item AgentMessageItem
		if err := jsonv2.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		if _, err := requiredJSONMembers(data, "text"); err != nil {
			return nil, err
		}
		return &item, nil
	case ItemReasoning:
		var item ReasoningItem
		if err := jsonv2.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		if _, err := requiredJSONMembers(data, "text"); err != nil {
			return nil, err
		}
		return &item, nil
	case ItemCommandExecution:
		var item CommandExecutionItem
		if err := jsonv2.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		if _, err := requiredJSONMembers(data, "command", "aggregated_output", "status"); err != nil {
			return nil, err
		}
		if !slices.Contains([]CommandExecutionStatus{CommandInProgress, CommandCompleted, CommandFailed, CommandDeclined}, item.Status) {
			return nil, protocolPayloadError(fmt.Sprintf("command_execution has invalid status %q", item.Status))
		}
		return &item, nil
	case ItemFileChange:
		var item FileChangeItem
		if err := jsonv2.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		members, err := requiredJSONMembers(data, "changes", "status")
		if err != nil {
			return nil, err
		}
		if !slices.Contains([]PatchApplyStatus{PatchApplyInProgress, PatchApplyCompleted, PatchApplyFailed}, item.Status) {
			return nil, protocolPayloadError(fmt.Sprintf("file_change has invalid status %q", item.Status))
		}
		var rawChanges []jsontext.Value
		if err := jsonv2.Unmarshal(members["changes"], &rawChanges); err != nil {
			return nil, err
		}
		for index, change := range item.Changes {
			if _, err := requiredJSONMembers(rawChanges[index], "path", "kind"); err != nil {
				return nil, fmt.Errorf("file_change change %d: %w", index, err)
			}
			if change.Path == "" || !slices.Contains([]PatchChangeKind{PatchChangeAdd, PatchChangeDelete, PatchChangeUpdate}, change.Kind) {
				return nil, protocolPayloadError("file_change has an invalid change")
			}
		}
		return &item, nil
	case ItemMCPToolCall:
		var item MCPToolCallItem
		if err := jsonv2.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		members, err := presentJSONMembers(data, "server", "tool", "arguments", "status")
		if err != nil {
			return nil, err
		}
		if isJSONNull(members["server"]) || isJSONNull(members["tool"]) || isJSONNull(members["status"]) || item.Server == "" || item.Tool == "" {
			return nil, protocolPayloadError("mcp_tool_call is missing server, tool, or arguments")
		}
		if !slices.Contains([]MCPToolCallStatus{MCPToolCallInProgress, MCPToolCallCompleted, MCPToolCallFailed}, item.Status) {
			return nil, protocolPayloadError(fmt.Sprintf("mcp_tool_call has invalid status %q", item.Status))
		}
		if item.Result != nil {
			resultMembers, err := presentJSONMembers(members["result"], "content", "structured_content")
			if err != nil {
				return nil, fmt.Errorf("mcp_tool_call result: %w", err)
			}
			if isJSONNull(resultMembers["content"]) {
				return nil, protocolPayloadError("mcp_tool_call result content must not be null")
			}
		}
		if item.Error != nil && item.Error.Message == "" {
			return nil, protocolPayloadError("mcp_tool_call error has an empty message")
		}
		cloneMCPPayloads(&item)
		return &item, nil
	case ItemWebSearch:
		var item WebSearchItem
		if err := jsonv2.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		if _, err := requiredJSONMembers(data, "query"); err != nil {
			return nil, err
		}
		return &item, nil
	case ItemTodoList:
		var item TodoListItem
		if err := jsonv2.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		members, err := requiredJSONMembers(data, "items")
		if err != nil {
			return nil, err
		}
		var rawItems []jsontext.Value
		if err := jsonv2.Unmarshal(members["items"], &rawItems); err != nil {
			return nil, err
		}
		for index, rawItem := range rawItems {
			if _, err := requiredJSONMembers(rawItem, "text", "completed"); err != nil {
				return nil, fmt.Errorf("todo_list item %d: %w", index, err)
			}
		}
		return &item, nil
	case ItemError:
		var item ErrorItem
		if err := jsonv2.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		if _, err := requiredJSONMembers(data, "message"); err != nil {
			return nil, err
		}
		if item.Message == "" {
			return nil, protocolPayloadError("error Thread Item has an empty message")
		}
		return &item, nil
	default:
		return &UnknownItem{
			ID:          header.ID,
			UnknownType: header.Type,
			Raw:         json.RawMessage(bytes.Clone(data)),
		}, nil
	}
}

func requiredJSONMembers(data []byte, names ...string) (map[string]jsontext.Value, error) {
	members, err := presentJSONMembers(data, names...)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if isJSONNull(members[name]) {
			return nil, protocolPayloadError(fmt.Sprintf("payload member %q must not be null", name))
		}
	}
	return members, nil
}

func presentJSONMembers(data []byte, names ...string) (map[string]jsontext.Value, error) {
	var members map[string]jsontext.Value
	if err := jsonv2.Unmarshal(data, &members); err != nil {
		return nil, err
	}
	for _, name := range names {
		if _, ok := members[name]; !ok {
			return nil, protocolPayloadError(fmt.Sprintf("payload is missing member %q", name))
		}
	}
	return members, nil
}

func isJSONNull(data []byte) bool {
	return bytes.Equal(bytes.TrimSpace(data), []byte("null"))
}

func cloneMCPPayloads(item *MCPToolCallItem) {
	item.Arguments = bytes.Clone(item.Arguments)
	if item.Result == nil {
		return
	}
	item.Result.Meta = bytes.Clone(item.Result.Meta)
	item.Result.StructuredContent = bytes.Clone(item.Result.StructuredContent)
	item.Result.Content = slices.Clone(item.Result.Content)
	for index := range item.Result.Content {
		item.Result.Content[index] = bytes.Clone(item.Result.Content[index])
	}
}

func protocolPayloadError(message string) error {
	return &ProtocolError{Message: message}
}
