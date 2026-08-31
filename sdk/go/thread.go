package codex

import (
	"bufio"
	"bytes"
	"context"
	stdjson "encoding/json"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"os/exec"
	"reflect"
	"slices"
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
	// Model selects the model used by this Thread.
	Model string
	// ThreadSource classifies a newly created Thread. It is not sent when a
	// Thread is resumed.
	ThreadSource string
	// SandboxMode controls filesystem and process isolation.
	SandboxMode SandboxMode
	// WorkingDirectory sets the Codex CLI working directory.
	WorkingDirectory string
	// AdditionalDirectories grants access to additional working directories.
	AdditionalDirectories []string
	// SkipGitRepoCheck allows the working directory to be outside a Git repository.
	SkipGitRepoCheck bool
	// ModelReasoningEffort selects the model reasoning effort.
	ModelReasoningEffort ModelReasoningEffort
	// NetworkAccess controls network access in the workspace-write sandbox.
	NetworkAccess NetworkAccessMode
	// WebSearchMode controls web search behavior.
	WebSearchMode WebSearchMode
	// ApprovalPolicy controls when Codex requests approval.
	ApprovalPolicy ApprovalPolicy
}

// SandboxMode identifies a Codex sandbox policy.
type SandboxMode string

const (
	// SandboxReadOnly allows reads but prevents workspace writes.
	SandboxReadOnly SandboxMode = "read-only"
	// SandboxWorkspaceWrite allows writes within approved workspace roots.
	SandboxWorkspaceWrite SandboxMode = "workspace-write"
	// SandboxDangerFullAccess disables sandbox restrictions.
	SandboxDangerFullAccess SandboxMode = "danger-full-access"
)

// ModelReasoningEffort identifies a supported model reasoning effort.
type ModelReasoningEffort string

const (
	// ReasoningEffortMinimal requests minimal reasoning.
	ReasoningEffortMinimal ModelReasoningEffort = "minimal"
	// ReasoningEffortLow requests low reasoning.
	ReasoningEffortLow ModelReasoningEffort = "low"
	// ReasoningEffortMedium requests medium reasoning.
	ReasoningEffortMedium ModelReasoningEffort = "medium"
	// ReasoningEffortHigh requests high reasoning.
	ReasoningEffortHigh ModelReasoningEffort = "high"
	// ReasoningEffortXHigh requests extra-high reasoning.
	ReasoningEffortXHigh ModelReasoningEffort = "xhigh"
	// ReasoningEffortMax requests maximum reasoning.
	ReasoningEffortMax ModelReasoningEffort = "max"
	// ReasoningEffortUltra requests ultra reasoning.
	ReasoningEffortUltra ModelReasoningEffort = "ultra"
	// ReasoningEffortPersistent requests persistent reasoning.
	ReasoningEffortPersistent ModelReasoningEffort = "persistent"
)

// NetworkAccessMode controls workspace network access. Its zero value inherits
// the Codex configuration.
type NetworkAccessMode string

const (
	// NetworkAccessEnabled enables workspace network access.
	NetworkAccessEnabled NetworkAccessMode = "enabled"
	// NetworkAccessDisabled disables workspace network access.
	NetworkAccessDisabled NetworkAccessMode = "disabled"
)

// WebSearchMode controls web search. Its zero value inherits Codex configuration.
type WebSearchMode string

const (
	// WebSearchDisabled disables web search.
	WebSearchDisabled WebSearchMode = "disabled"
	// WebSearchCached allows cached web search results.
	WebSearchCached WebSearchMode = "cached"
	// WebSearchLive allows live web searches.
	WebSearchLive WebSearchMode = "live"
)

// ApprovalPolicy controls when Codex asks for approval.
type ApprovalPolicy string

const (
	// ApprovalNever never requests approval.
	ApprovalNever ApprovalPolicy = "never"
	// ApprovalOnRequest requests approval when the agent decides it is needed.
	ApprovalOnRequest ApprovalPolicy = "on-request"
	// ApprovalOnFailure requests approval after a sandboxed command fails.
	ApprovalOnFailure ApprovalPolicy = "on-failure"
	// ApprovalUntrusted requests approval for untrusted commands.
	ApprovalUntrusted ApprovalPolicy = "untrusted"
)

// TurnOptions configures one Turn.
type TurnOptions struct {
	// OutputSchema is a JSON Schema that constrains the agent's final response.
	// It must be a valid top-level JSON object.
	OutputSchema stdjson.RawMessage
}

// StructuredInput combines prompt text with local image paths.
type StructuredInput struct {
	// Text is written to the Codex CLI standard input.
	Text string
	// LocalImages contains image paths forwarded as repeated --image arguments.
	LocalImages []string
}

// TurnInput is input accepted by Run and RunStreamed.
type TurnInput interface {
	~string | StructuredInput
}

type normalizedTurnInput struct {
	prompt string
	images []string
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

// StructuredTurn contains both the ordinary Turn and its decoded output.
type StructuredTurn[T any] struct {
	Turn   Turn
	Output T
}

// RunJSON runs a Turn with schema-constrained output and decodes the final
// response into T. The input type is inferred when callers specify T.
func (t *Thread) RunJSON[T any, I TurnInput](
	ctx context.Context,
	input I,
	schema stdjson.RawMessage,
	options TurnOptions,
) (StructuredTurn[T], error) {
	if schema == nil {
		return StructuredTurn[T]{}, &ValidationError{
			Field: "output schema",
			Err:   errors.New("must be a valid top-level JSON object"),
		}
	}
	if options.OutputSchema != nil {
		return StructuredTurn[T]{}, &ValidationError{
			Field: "output schema",
			Err:   errors.New("must be provided either as the RunJSON schema or TurnOptions.OutputSchema, not both"),
		}
	}
	options.OutputSchema = bytes.Clone(schema)
	turn, err := t.Run(ctx, input, options)
	if err != nil {
		return StructuredTurn[T]{}, err
	}
	var output T
	if err := json.Unmarshal([]byte(turn.FinalResponse), &output, json.RejectUnknownMembers(true)); err != nil {
		return StructuredTurn[T]{}, &OutputDecodeError{
			Target: reflect.TypeFor[T]().String(),
			Err:    err,
		}
	}
	return StructuredTurn[T]{Turn: turn, Output: output}, nil
}

// RunStreamed provides input to the agent and lazily streams Thread Events.
// The returned iterator is single-use. Iteration owns the CLI process and
// synchronously cleans it up when the caller stops early.
func (t *Thread) RunStreamed[I TurnInput](
	ctx context.Context,
	input I,
	options TurnOptions,
) iter.Seq2[ThreadEvent, error] {
	normalized := normalizeInput(input)
	options.OutputSchema = bytes.Clone(options.OutputSchema)
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
		t.execute(ctx, normalized, options, yield)
	}
}

func (t *Thread) execute(
	ctx context.Context,
	input normalizedTurnInput,
	options TurnOptions,
	yield func(ThreadEvent, error) bool,
) {
	schemaPath, cleanupSchema, err := createOutputSchemaFile(options.OutputSchema)
	if err != nil {
		yield(nil, err)
		return
	}
	defer cleanupSchema()

	args := []string{"exec", "--experimental-json"}
	for _, override := range t.client.configOverrides {
		args = append(args, "--config", override)
	}
	if t.client.baseURL != "" {
		value, _ := renderTOMLValue(t.client.baseURL, "openai_base_url")
		args = append(args, "--config", "openai_base_url="+value)
	}
	threadID, resumed := t.ID()
	if t.options.Model != "" {
		args = append(args, "--model", t.options.Model)
	}
	if t.options.ThreadSource != "" && !resumed {
		args = append(args, "--thread-source", t.options.ThreadSource)
	}
	if t.options.SandboxMode != "" {
		args = append(args, "--sandbox", string(t.options.SandboxMode))
	}
	if t.options.WorkingDirectory != "" {
		args = append(args, "--cd", t.options.WorkingDirectory)
	}
	for _, directory := range t.options.AdditionalDirectories {
		args = append(args, "--add-dir", directory)
	}
	if t.options.SkipGitRepoCheck {
		args = append(args, "--skip-git-repo-check")
	}
	if schemaPath != "" {
		args = append(args, "--output-schema", schemaPath)
	}
	if t.options.ModelReasoningEffort != "" {
		args = append(args, "--config", `model_reasoning_effort="`+string(t.options.ModelReasoningEffort)+`"`)
	}
	switch t.options.NetworkAccess {
	case NetworkAccessEnabled:
		args = append(args, "--config", "sandbox_workspace_write.network_access=true")
	case NetworkAccessDisabled:
		args = append(args, "--config", "sandbox_workspace_write.network_access=false")
	}
	if t.options.WebSearchMode != "" {
		args = append(args, "--config", `web_search="`+string(t.options.WebSearchMode)+`"`)
	}
	if t.options.ApprovalPolicy != "" {
		args = append(args, "--config", `approval_policy="`+string(t.options.ApprovalPolicy)+`"`)
	}
	if resumed {
		args = append(args, "resume", threadID)
	}
	for _, image := range input.images {
		args = append(args, "--image", image)
	}
	command := exec.CommandContext(ctx, t.client.executable, args...)
	command.Env = t.client.environment
	command.Stdin = bytes.NewBufferString(input.prompt)
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

func normalizeInput[I TurnInput](input I) normalizedTurnInput {
	if structured, ok := any(input).(StructuredInput); ok {
		return normalizedTurnInput{
			prompt: strings.Clone(structured.Text),
			images: slices.Clone(structured.LocalImages),
		}
	}
	return normalizedTurnInput{prompt: strings.Clone(reflect.ValueOf(input).String())}
}

func snapshotThreadOptions(options ThreadOptions) ThreadOptions {
	options.AdditionalDirectories = slices.Clone(options.AdditionalDirectories)
	return options
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
