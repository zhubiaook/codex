package codex

import "fmt"

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

func (e *ProtocolError) Error() string {
	return "codex: invalid event sequence: " + e.Message
}
