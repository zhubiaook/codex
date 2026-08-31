package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os/exec"
)

const (
	maxEventBytes   = 16 << 20
	maxErrorPreview = 4 << 10
	maxStderrBytes  = 64 << 10
)

// ThreadOptions configures a Thread.
type ThreadOptions struct{}

// TurnOptions configures one Turn.
type TurnOptions struct{}

// TurnInput is input accepted by Run.
type TurnInput interface {
	~string
}

// Thread is a persisted conversation with the Codex agent.
type Thread struct {
	client  *Client
	options ThreadOptions
}

// Run provides input to the agent and returns the completed Turn.
func (t *Thread) Run[I TurnInput](ctx context.Context, input I, options TurnOptions) (Turn, error) {
	command := exec.CommandContext(ctx, t.client.executable, "exec", "--experimental-json")
	command.Env = t.client.environment
	command.Stdin = bytes.NewBufferString(string(input))
	stdout, err := command.StdoutPipe()
	if err != nil {
		return Turn{}, &ExecError{Path: t.client.executable, ExitCode: -1, Err: err}
	}
	stderr := newBoundedBuffer(maxStderrBytes)
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		return Turn{}, &ExecError{Path: t.client.executable, ExitCode: -1, Err: err}
	}

	turn := Turn{Items: []ThreadItem{}}
	completed := false
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), maxEventBytes)
	line := 0
	for scanner.Scan() {
		line++
		if err := consumeEvent(scanner.Bytes(), &turn, &completed); err != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			return Turn{}, &DecodeError{
				Line:    line,
				Preview: preview(scanner.Bytes()),
				Err:     err,
			}
		}
	}
	if err := scanner.Err(); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return Turn{}, &DecodeError{Line: line + 1, Err: err}
	}

	waitErr := command.Wait()
	if ctx.Err() != nil {
		return Turn{}, ctx.Err()
	}
	if waitErr != nil {
		exitCode := -1
		if exitError, ok := errors.AsType[*exec.ExitError](waitErr); ok {
			exitCode = exitError.ExitCode()
		}
		return Turn{}, &ExecError{
			Path:     t.client.executable,
			ExitCode: exitCode,
			Stderr:   stderr.String(),
			Err:      waitErr,
		}
	}
	if !completed {
		return Turn{}, &ProtocolError{Message: "process exited without turn.completed"}
	}
	return turn, nil
}

type eventHeader struct {
	Type string `json:"type"`
}

type itemCompletedEvent struct {
	Item jsontext.Value `json:"item"`
}

type itemHeader struct {
	Type string `json:"type"`
}

type turnCompletedEvent struct {
	Usage Usage `json:"usage"`
}

func consumeEvent(data []byte, turn *Turn, completed *bool) error {
	var header eventHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return err
	}
	switch header.Type {
	case "thread.started", "turn.started":
		return nil
	case "item.completed":
		var event itemCompletedEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		return consumeCompletedItem(event.Item, turn)
	case "turn.completed":
		var event turnCompletedEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		turn.Usage = new(event.Usage)
		*completed = true
		return nil
	default:
		return fmt.Errorf("unsupported event type %q", header.Type)
	}
}

func consumeCompletedItem(data []byte, turn *Turn) error {
	var header itemHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return err
	}
	switch header.Type {
	case "agent_message":
		var item AgentMessageItem
		if err := json.Unmarshal(data, &item); err != nil {
			return err
		}
		turn.Items = append(turn.Items, &item)
		turn.FinalResponse = item.Text
		return nil
	default:
		return fmt.Errorf("unsupported item type %q", header.Type)
	}
}

func preview(data []byte) string {
	return string(data[:min(len(data), maxErrorPreview)])
}

type boundedBuffer struct {
	buffer    []byte
	limit     int
	truncated bool
}

func newBoundedBuffer(limit int) *boundedBuffer {
	return &boundedBuffer{buffer: make([]byte, 0, limit), limit: limit}
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	written := len(data)
	remaining := b.limit - len(b.buffer)
	if remaining > 0 {
		b.buffer = append(b.buffer, data[:min(len(data), remaining)]...)
	}
	if len(data) > remaining {
		b.truncated = true
	}
	return written, nil
}

func (b *boundedBuffer) String() string {
	if b.truncated {
		return string(b.buffer) + "\n[stderr truncated]"
	}
	return string(b.buffer)
}
