package codex

import stdjson "encoding/json"

// EventType identifies a Thread Event on the Codex CLI JSONL wire.
type EventType string

const (
	// EventThreadStarted identifies a ThreadStartedEvent.
	EventThreadStarted EventType = "thread.started"
	// EventTurnStarted identifies a TurnStartedEvent.
	EventTurnStarted EventType = "turn.started"
	// EventTurnCompleted identifies a TurnCompletedEvent.
	EventTurnCompleted EventType = "turn.completed"
	// EventTurnFailed identifies a TurnFailedEvent.
	EventTurnFailed EventType = "turn.failed"
	// EventItemStarted identifies an ItemStartedEvent.
	EventItemStarted EventType = "item.started"
	// EventItemUpdated identifies an ItemUpdatedEvent.
	EventItemUpdated EventType = "item.updated"
	// EventItemCompleted identifies an ItemCompletedEvent.
	EventItemCompleted EventType = "item.completed"
	// EventError identifies a ThreadErrorEvent.
	EventError EventType = "error"
)

// ThreadEvent is a lifecycle notification emitted while a Turn runs.
type ThreadEvent interface {
	EventType() EventType
	isThreadEvent()
}

// ThreadStartedEvent reports the identifier of a newly started Thread.
type ThreadStartedEvent struct {
	ThreadID string `json:"thread_id"`
}

// EventType returns EventThreadStarted.
func (*ThreadStartedEvent) EventType() EventType { return EventThreadStarted }
func (*ThreadStartedEvent) isThreadEvent()       {}

// TurnStartedEvent reports that a Turn has started.
type TurnStartedEvent struct{}

// EventType returns EventTurnStarted.
func (*TurnStartedEvent) EventType() EventType { return EventTurnStarted }
func (*TurnStartedEvent) isThreadEvent()       {}

// TurnCompletedEvent reports that a Turn completed successfully.
type TurnCompletedEvent struct {
	Usage Usage `json:"usage"`
}

// EventType returns EventTurnCompleted.
func (*TurnCompletedEvent) EventType() EventType { return EventTurnCompleted }
func (*TurnCompletedEvent) isThreadEvent()       {}

// ThreadError describes a failure reported by the Codex CLI.
type ThreadError struct {
	Message string `json:"message"`
}

// TurnFailedEvent reports that a Turn failed.
type TurnFailedEvent struct {
	Error ThreadError `json:"error"`
}

// EventType returns EventTurnFailed.
func (*TurnFailedEvent) EventType() EventType { return EventTurnFailed }
func (*TurnFailedEvent) isThreadEvent()       {}

// ItemStartedEvent reports a newly started Thread Item.
type ItemStartedEvent struct {
	Item ThreadItem
}

// EventType returns EventItemStarted.
func (*ItemStartedEvent) EventType() EventType { return EventItemStarted }
func (*ItemStartedEvent) isThreadEvent()       {}

// ItemUpdatedEvent reports an updated Thread Item.
type ItemUpdatedEvent struct {
	Item ThreadItem
}

// EventType returns EventItemUpdated.
func (*ItemUpdatedEvent) EventType() EventType { return EventItemUpdated }
func (*ItemUpdatedEvent) isThreadEvent()       {}

// ItemCompletedEvent reports a completed Thread Item.
type ItemCompletedEvent struct {
	Item ThreadItem
}

// EventType returns EventItemCompleted.
func (*ItemCompletedEvent) EventType() EventType { return EventItemCompleted }
func (*ItemCompletedEvent) isThreadEvent()       {}

// ThreadErrorEvent reports an unrecoverable stream error.
type ThreadErrorEvent struct {
	Message string `json:"message"`
}

// EventType returns EventError.
func (*ThreadErrorEvent) EventType() EventType { return EventError }
func (*ThreadErrorEvent) isThreadEvent()       {}

// Error returns the stream error message.
func (e *ThreadErrorEvent) Error() string { return "codex: Thread Event error: " + e.Message }

// UnknownEvent preserves an unrecognized Thread Event for forward compatibility.
type UnknownEvent struct {
	UnknownType EventType
	Raw         stdjson.RawMessage
}

// EventType returns the unrecognized wire discriminator.
func (e *UnknownEvent) EventType() EventType { return e.UnknownType }
func (*UnknownEvent) isThreadEvent()         {}
