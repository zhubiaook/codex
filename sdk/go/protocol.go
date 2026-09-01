package codex

import (
	"bytes"
	stdjson "encoding/json"
	"encoding/json/jsontext"
	json "encoding/json/v2"
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
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, err
	}
	if header.Type == "" {
		return nil, protocolPayloadError("Thread Event has no type discriminator")
	}
	switch header.Type {
	case EventThreadStarted:
		var event ThreadStartedEvent
		if err := json.Unmarshal(data, &event); err != nil {
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
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		return &event, nil
	case EventTurnFailed:
		var event TurnFailedEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		if event.Error.Message == "" {
			return nil, protocolPayloadError("turn.failed has an empty error message")
		}
		return &event, nil
	case EventItemStarted, EventItemUpdated, EventItemCompleted:
		var wire itemEventWire
		if err := json.Unmarshal(data, &wire); err != nil {
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
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, err
		}
		if event.Message == "" {
			return nil, protocolPayloadError("error event has an empty message")
		}
		return &event, nil
	default:
		return &UnknownEvent{UnknownType: header.Type, Raw: stdjson.RawMessage(bytes.Clone(data))}, nil
	}
	return nil, protocolPayloadError(fmt.Sprintf("unsupported Thread Event %q", header.Type))
}

func decodeThreadItem(data []byte) (ThreadItem, error) {
	var header itemHeader
	if err := json.Unmarshal(data, &header); err != nil {
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
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		return &item, nil
	case ItemReasoning:
		var item ReasoningItem
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		return &item, nil
	case ItemCommandExecution:
		var item CommandExecutionItem
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		if !slices.Contains([]CommandExecutionStatus{CommandInProgress, CommandCompleted, CommandFailed}, item.Status) {
			return nil, protocolPayloadError(fmt.Sprintf("command_execution has invalid status %q", item.Status))
		}
		return &item, nil
	case ItemFileChange:
		var item FileChangeItem
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		if !slices.Contains([]PatchApplyStatus{PatchApplyCompleted, PatchApplyFailed}, item.Status) {
			return nil, protocolPayloadError(fmt.Sprintf("file_change has invalid status %q", item.Status))
		}
		for _, change := range item.Changes {
			if change.Path == "" || !slices.Contains([]PatchChangeKind{PatchChangeAdd, PatchChangeDelete, PatchChangeUpdate}, change.Kind) {
				return nil, protocolPayloadError("file_change has an invalid change")
			}
		}
		return &item, nil
	case ItemMCPToolCall:
		var item MCPToolCallItem
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		if item.Server == "" || item.Tool == "" || item.Arguments == nil {
			return nil, protocolPayloadError("mcp_tool_call is missing server, tool, or arguments")
		}
		if !slices.Contains([]MCPToolCallStatus{MCPToolCallInProgress, MCPToolCallCompleted, MCPToolCallFailed}, item.Status) {
			return nil, protocolPayloadError(fmt.Sprintf("mcp_tool_call has invalid status %q", item.Status))
		}
		cloneMCPPayloads(&item)
		return &item, nil
	case ItemWebSearch:
		var item WebSearchItem
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		return &item, nil
	case ItemTodoList:
		var item TodoListItem
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		return &item, nil
	case ItemError:
		var item ErrorItem
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		return &item, nil
	default:
		return &UnknownItem{
			ID:          header.ID,
			UnknownType: header.Type,
			Raw:         stdjson.RawMessage(bytes.Clone(data)),
		}, nil
	}
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
