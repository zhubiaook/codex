package codex

import "encoding/json"

// ItemType identifies a Thread Item on the Codex CLI JSONL wire.
type ItemType string

const (
	// ItemAgentMessage identifies an AgentMessageItem.
	ItemAgentMessage ItemType = "agent_message"
	// ItemReasoning identifies a ReasoningItem.
	ItemReasoning ItemType = "reasoning"
	// ItemCommandExecution identifies a CommandExecutionItem.
	ItemCommandExecution ItemType = "command_execution"
	// ItemFileChange identifies a FileChangeItem.
	ItemFileChange ItemType = "file_change"
	// ItemMCPToolCall identifies an MCPToolCallItem.
	ItemMCPToolCall ItemType = "mcp_tool_call"
	// ItemWebSearch identifies a WebSearchItem.
	ItemWebSearch ItemType = "web_search"
	// ItemTodoList identifies a TodoListItem.
	ItemTodoList ItemType = "todo_list"
	// ItemError identifies an ErrorItem.
	ItemError ItemType = "error"
)

// ThreadItem is a structured unit of agent activity produced during a Turn.
type ThreadItem interface {
	ItemType() ItemType
	isThreadItem()
}

// AgentMessageItem is a natural-language or structured response from the agent.
type AgentMessageItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// ItemType returns ItemAgentMessage.
func (*AgentMessageItem) ItemType() ItemType { return ItemAgentMessage }
func (*AgentMessageItem) isThreadItem()      {}

// ReasoningItem is an agent reasoning summary.
type ReasoningItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// ItemType returns ItemReasoning.
func (*ReasoningItem) ItemType() ItemType { return ItemReasoning }
func (*ReasoningItem) isThreadItem()      {}

// CommandExecutionStatus is the lifecycle state of a command execution.
type CommandExecutionStatus string

const (
	// CommandInProgress indicates that a command is running.
	CommandInProgress CommandExecutionStatus = "in_progress"
	// CommandCompleted indicates that a command completed successfully.
	CommandCompleted CommandExecutionStatus = "completed"
	// CommandFailed indicates that a command failed.
	CommandFailed CommandExecutionStatus = "failed"
	// CommandDeclined indicates that a command was not approved to run.
	CommandDeclined CommandExecutionStatus = "declined"
)

// CommandExecutionItem describes a command executed by the agent.
type CommandExecutionItem struct {
	ID               string                 `json:"id"`
	Command          string                 `json:"command"`
	AggregatedOutput string                 `json:"aggregated_output"`
	ExitCode         *int                   `json:"exit_code"`
	Status           CommandExecutionStatus `json:"status"`
}

// ItemType returns ItemCommandExecution.
func (*CommandExecutionItem) ItemType() ItemType { return ItemCommandExecution }
func (*CommandExecutionItem) isThreadItem()      {}

// PatchChangeKind identifies how a file changed.
type PatchChangeKind string

const (
	// PatchChangeAdd identifies a newly added file.
	PatchChangeAdd PatchChangeKind = "add"
	// PatchChangeDelete identifies a deleted file.
	PatchChangeDelete PatchChangeKind = "delete"
	// PatchChangeUpdate identifies an updated file.
	PatchChangeUpdate PatchChangeKind = "update"
)

// FileUpdateChange describes one file changed by a patch.
type FileUpdateChange struct {
	Path string          `json:"path"`
	Kind PatchChangeKind `json:"kind"`
}

// PatchApplyStatus is the lifecycle state of a file change.
type PatchApplyStatus string

const (
	// PatchApplyInProgress indicates that the patch is being applied.
	PatchApplyInProgress PatchApplyStatus = "in_progress"
	// PatchApplyCompleted indicates that the patch was applied.
	PatchApplyCompleted PatchApplyStatus = "completed"
	// PatchApplyFailed indicates that the patch failed.
	PatchApplyFailed PatchApplyStatus = "failed"
)

// FileChangeItem describes a set of file changes made by the agent.
type FileChangeItem struct {
	ID      string             `json:"id"`
	Changes []FileUpdateChange `json:"changes"`
	Status  PatchApplyStatus   `json:"status"`
}

// ItemType returns ItemFileChange.
func (*FileChangeItem) ItemType() ItemType { return ItemFileChange }
func (*FileChangeItem) isThreadItem()      {}

// MCPToolCallStatus is the lifecycle state of an MCP tool call.
type MCPToolCallStatus string

const (
	// MCPToolCallInProgress indicates that an MCP tool call is running.
	MCPToolCallInProgress MCPToolCallStatus = "in_progress"
	// MCPToolCallCompleted indicates that an MCP tool call completed.
	MCPToolCallCompleted MCPToolCallStatus = "completed"
	// MCPToolCallFailed indicates that an MCP tool call failed.
	MCPToolCallFailed MCPToolCallStatus = "failed"
)

// MCPToolCallResult contains a successful MCP tool result without imposing an
// MCP SDK dependency on callers.
type MCPToolCallResult struct {
	Content           []json.RawMessage `json:"content"`
	Meta              json.RawMessage   `json:"_meta"`
	StructuredContent json.RawMessage   `json:"structured_content"`
}

// MCPToolCallError describes a failed MCP tool call.
type MCPToolCallError struct {
	Message string `json:"message"`
}

// MCPToolCallItem describes a call to an MCP server tool.
type MCPToolCallItem struct {
	ID        string             `json:"id"`
	Server    string             `json:"server"`
	Tool      string             `json:"tool"`
	Arguments json.RawMessage    `json:"arguments"`
	Result    *MCPToolCallResult `json:"result"`
	Error     *MCPToolCallError  `json:"error"`
	Status    MCPToolCallStatus  `json:"status"`
}

// ItemType returns ItemMCPToolCall.
func (*MCPToolCallItem) ItemType() ItemType { return ItemMCPToolCall }
func (*MCPToolCallItem) isThreadItem()      {}

// WebSearchItem describes a web search requested by the agent.
type WebSearchItem struct {
	ID    string `json:"id"`
	Query string `json:"query"`
}

// ItemType returns ItemWebSearch.
func (*WebSearchItem) ItemType() ItemType { return ItemWebSearch }
func (*WebSearchItem) isThreadItem()      {}

// TodoItem describes one step in an agent plan.
type TodoItem struct {
	Text      string `json:"text"`
	Completed bool   `json:"completed"`
}

// TodoListItem contains the agent's current plan.
type TodoListItem struct {
	ID    string     `json:"id"`
	Items []TodoItem `json:"items"`
}

// ItemType returns ItemTodoList.
func (*TodoListItem) ItemType() ItemType { return ItemTodoList }
func (*TodoListItem) isThreadItem()      {}

// ErrorItem is a non-fatal error surfaced as a Thread Item.
type ErrorItem struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// ItemType returns ItemError.
func (*ErrorItem) ItemType() ItemType { return ItemError }
func (*ErrorItem) isThreadItem()      {}

// UnknownItem preserves an unrecognized Thread Item for forward compatibility.
type UnknownItem struct {
	ID          string
	UnknownType ItemType
	Raw         json.RawMessage
}

// ItemType returns the unrecognized wire discriminator.
func (i *UnknownItem) ItemType() ItemType { return i.UnknownType }
func (*UnknownItem) isThreadItem()        {}

// Usage describes token usage during a Turn.
type Usage struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens int64 `json:"cache_write_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
}

// Turn is a completed request-and-response cycle within a Thread.
type Turn struct {
	Items         []ThreadItem
	FinalResponse string
	Usage         *Usage
}
