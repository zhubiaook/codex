package codex

// EventType identifies a Thread Event on the Codex CLI JSONL wire.
type EventType string

const (
	// EventThreadStarted identifies a ThreadStartedEvent.
	EventThreadStarted EventType = "thread.started"
	// EventTurnStarted identifies a TurnStartedEvent.
	EventTurnStarted EventType = "turn.started"
	// EventItemCompleted identifies an ItemCompletedEvent.
	EventItemCompleted EventType = "item.completed"
	// EventTurnCompleted identifies a TurnCompletedEvent.
	EventTurnCompleted EventType = "turn.completed"
)

// ThreadEvent is a lifecycle notification emitted while a Turn runs.
type ThreadEvent interface {
	EventType() EventType
	isThreadEvent()
}

// ThreadStartedEvent reports the identifier of a newly started Thread.
type ThreadStartedEvent struct {
	ThreadID string
}

// EventType returns EventThreadStarted.
func (*ThreadStartedEvent) EventType() EventType { return EventThreadStarted }
func (*ThreadStartedEvent) isThreadEvent()       {}

// TurnStartedEvent reports that a Turn has started.
type TurnStartedEvent struct{}

// EventType returns EventTurnStarted.
func (*TurnStartedEvent) EventType() EventType { return EventTurnStarted }
func (*TurnStartedEvent) isThreadEvent()       {}

// ItemCompletedEvent reports a completed Thread Item.
type ItemCompletedEvent struct {
	Item ThreadItem
}

// EventType returns EventItemCompleted.
func (*ItemCompletedEvent) EventType() EventType { return EventItemCompleted }
func (*ItemCompletedEvent) isThreadEvent()       {}

// TurnCompletedEvent reports that a Turn completed successfully.
type TurnCompletedEvent struct {
	Usage Usage
}

// EventType returns EventTurnCompleted.
func (*TurnCompletedEvent) EventType() EventType { return EventTurnCompleted }
func (*TurnCompletedEvent) isThreadEvent()       {}
