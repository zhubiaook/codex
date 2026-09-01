package codex

import (
	"errors"
	"fmt"
)

var (
	// ErrTurnInProgress indicates that the Thread already has an active Turn.
	ErrTurnInProgress = errors.New("codex: turn already in progress")
	// ErrStreamConsumed indicates that a Thread Event stream was already used.
	ErrStreamConsumed = errors.New("codex: event stream already consumed")
)

// ValidationError reports invalid SDK input or options.
type ValidationError struct {
	Field string
	Err   error
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("codex: invalid %s: %v", e.Field, e.Err)
}

// Unwrap returns the underlying validation error.
func (e *ValidationError) Unwrap() error {
	return e.Err
}

// ExecutableError reports that the Codex CLI executable could not be resolved.
type ExecutableError struct {
	Path string
	Err  error
}

func (e *ExecutableError) Error() string {
	return fmt.Sprintf("codex: resolve executable %q: %v", e.Path, e.Err)
}

// Unwrap returns the underlying executable resolution error.
func (e *ExecutableError) Unwrap() error {
	return e.Err
}

// ExecError reports that a Codex CLI process could not run successfully.
type ExecError struct {
	Path     string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *ExecError) Error() string {
	if e.Stderr == "" {
		return fmt.Sprintf("codex: execute %q: %v", e.Path, e.Err)
	}
	return fmt.Sprintf("codex: execute %q: %v: %s", e.Path, e.Err, e.Stderr)
}

// Unwrap returns the underlying process error.
func (e *ExecError) Unwrap() error {
	return e.Err
}

// DecodeError reports invalid JSONL output from the Codex CLI.
type DecodeError struct {
	Line    int
	Preview string
	Err     error
}

func (e *DecodeError) Error() string {
	return fmt.Sprintf("codex: decode JSONL line %d: %v; preview: %q", e.Line, e.Err, e.Preview)
}

// Unwrap returns the underlying decoding error.
func (e *DecodeError) Unwrap() error {
	return e.Err
}

// ProtocolError reports an invalid Codex CLI event sequence.
type ProtocolError struct {
	Message string
}

// OutputDecodeError reports that a structured final response could not be
// decoded into the requested Go type.
type OutputDecodeError struct {
	Target string
	Err    error
}

func (e *OutputDecodeError) Error() string {
	return fmt.Sprintf("codex: decode final response into %s: %v", e.Target, e.Err)
}

// Unwrap returns the underlying JSON decoding error.
func (e *OutputDecodeError) Unwrap() error {
	return e.Err
}

// TurnFailedError reports a turn.failed Thread Event.
type TurnFailedError struct {
	ThreadError ThreadError
}

func (e *TurnFailedError) Error() string {
	return "codex: Turn failed: " + e.ThreadError.Message
}

func (e *ProtocolError) Error() string {
	return "codex: invalid event sequence: " + e.Message
}
