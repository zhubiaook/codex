package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	maxEventBytes   = 16 << 20
	maxErrorPreview = 4 << 10
	maxStderrBytes  = 64 << 10
)

// ThreadOptions configures a Thread.
type ThreadOptions struct {
	// ThreadSource classifies a newly created Thread. It is not sent when a
	// Thread is resumed.
	ThreadSource string
}

// TurnOptions configures one Turn.
type TurnOptions struct{}

// TurnInput is input accepted by Run and RunStreamed.
type TurnInput interface {
	~string
}

// Thread is a persisted conversation with the Codex agent.
type Thread struct {
	client  *Client
	options ThreadOptions
	mu      sync.RWMutex
	id      string
	active  atomic.Bool
}

// ID returns the Thread identifier after it has been established by the CLI.
func (t *Thread) ID() (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.id, t.id != ""
}

// Run provides input to the agent and returns the completed Turn.
func (t *Thread) Run[I TurnInput](ctx context.Context, input I, options TurnOptions) (Turn, error) {
	turn := Turn{Items: []ThreadItem{}}
	for event, err := range t.RunStreamed(ctx, input, options) {
		if err != nil {
			return Turn{}, err
		}
		switch event := event.(type) {
		case *ItemCompletedEvent:
			turn.Items = append(turn.Items, event.Item)
			if item, ok := event.Item.(*AgentMessageItem); ok {
				turn.FinalResponse = item.Text
			}
		case *TurnCompletedEvent:
			turn.Usage = new(event.Usage)
		}
	}
	return turn, nil
}

// RunStreamed provides input to the agent and lazily streams Thread Events.
// The returned iterator is single-use. Iteration owns the CLI process and
// synchronously cleans it up when the caller stops early.
func (t *Thread) RunStreamed[I TurnInput](
	ctx context.Context,
	input I,
	options TurnOptions,
) iter.Seq2[ThreadEvent, error] {
	prompt := strings.Clone(string(input))
	var consumed atomic.Bool
	return func(yield func(ThreadEvent, error) bool) {
		if !consumed.CompareAndSwap(false, true) {
			yield(nil, ErrStreamConsumed)
			return
		}
		if !t.active.CompareAndSwap(false, true) {
			yield(nil, ErrTurnInProgress)
			return
		}
		defer t.active.Store(false)
		t.execute(ctx, prompt, options, yield)
	}
}

func (t *Thread) execute(
	ctx context.Context,
	prompt string,
	options TurnOptions,
	yield func(ThreadEvent, error) bool,
) {
	args := []string{"exec", "--experimental-json"}
	threadID, resumed := t.ID()
	if t.options.ThreadSource != "" && !resumed {
		args = append(args, "--thread-source", t.options.ThreadSource)
	}
	if resumed {
		args = append(args, "resume", threadID)
	}
	command := exec.CommandContext(ctx, t.client.executable, args...)
	command.Env = t.client.environment
	command.Stdin = bytes.NewBufferString(prompt)
	stdout, err := command.StdoutPipe()
	if err != nil {
		yield(nil, &ExecError{Path: t.client.executable, ExitCode: -1, Err: err})
		return
	}
	stderr := newBoundedBuffer(maxStderrBytes)
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		yield(nil, &ExecError{Path: t.client.executable, ExitCode: -1, Err: err})
		return
	}

	waited := false
	defer func() {
		if waited {
			return
		}
		_ = command.Process.Kill()
		_ = command.Wait()
	}()

	completed := false
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64<<10), maxEventBytes)
	line := 0
	for scanner.Scan() {
		line++
		event, err := t.decodeEvent(scanner.Bytes())
		if err != nil {
			yield(nil, &DecodeError{
				Line:    line,
				Preview: preview(scanner.Bytes()),
				Err:     err,
			})
			return
		}
		if _, ok := event.(*TurnCompletedEvent); ok {
			completed = true
		}
		if !yield(event, nil) {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		yield(nil, &DecodeError{Line: line + 1, Err: err})
		return
	}

	waitErr := command.Wait()
	waited = true
	if ctx.Err() != nil {
		yield(nil, ctx.Err())
		return
	}
	if waitErr != nil {
		exitCode := -1
		if exitError, ok := errors.AsType[*exec.ExitError](waitErr); ok {
			exitCode = exitError.ExitCode()
		}
		yield(nil, &ExecError{
			Path:     t.client.executable,
			ExitCode: exitCode,
			Stderr:   stderr.String(),
			Err:      waitErr,
		})
		return
	}
	if !completed {
		yield(nil, &ProtocolError{Message: "process exited without turn.completed"})
	}
}

type eventHeader struct {
	Type string `json:"type"`
}

type threadStartedWireEvent struct {
	ThreadID string `json:"thread_id"`
}

type itemCompletedWireEvent struct {
	Item jsontext.Value `json:"item"`
}

type itemHeader struct {
	Type string `json:"type"`
}

type turnCompletedWireEvent struct {
	Usage Usage `json:"usage"`
}

func (t *Thread) decodeEvent(data []byte) (ThreadEvent, error) {
	var header eventHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, err
	}
	switch header.Type {
	case "thread.started":
		var wire threadStartedWireEvent
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, err
		}
		if wire.ThreadID == "" {
			return nil, errors.New("thread.started event has an empty thread_id")
		}
		t.mu.Lock()
		t.id = wire.ThreadID
		t.mu.Unlock()
		return &ThreadStartedEvent{ThreadID: wire.ThreadID}, nil
	case "turn.started":
		return &TurnStartedEvent{}, nil
	case "item.completed":
		var wire itemCompletedWireEvent
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, err
		}
		item, err := decodeCompletedItem(wire.Item)
		if err != nil {
			return nil, err
		}
		return &ItemCompletedEvent{Item: item}, nil
	case "turn.completed":
		var wire turnCompletedWireEvent
		if err := json.Unmarshal(data, &wire); err != nil {
			return nil, err
		}
		return &TurnCompletedEvent{Usage: wire.Usage}, nil
	default:
		return nil, fmt.Errorf("unsupported event type %q", header.Type)
	}
}

func decodeCompletedItem(data []byte) (ThreadItem, error) {
	var header itemHeader
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, err
	}
	switch header.Type {
	case "agent_message":
		var item AgentMessageItem
		if err := json.Unmarshal(data, &item); err != nil {
			return nil, err
		}
		return &item, nil
	default:
		return nil, fmt.Errorf("unsupported item type %q", header.Type)
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
