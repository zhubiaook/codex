package codex

// ThreadItem is a structured unit of agent activity produced during a Turn.
type ThreadItem interface {
	isThreadItem()
}

// AgentMessageItem is a natural-language or structured response from the agent.
type AgentMessageItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func (*AgentMessageItem) isThreadItem() {}

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
