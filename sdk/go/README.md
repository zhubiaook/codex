# Codex Go SDK

Embed the Codex agent in Go applications and workflows. The SDK wraps an
installed `codex` CLI, sends Turn input through standard input, and exposes the
CLI JSONL protocol as typed Go Thread Events and Thread Items.

## Requirements and installation

- Go 1.27 or newer
- A compatible Codex CLI available on `PATH`, or an explicit executable path

```console
go get github.com/zhubiaook/codex/sdk/go
```

Tags for this nested Go module use the `sdk/go/vX.Y.Z` convention.

## Quickstart

```go
client, err := codex.NewClient(codex.ClientOptions{})
if err != nil {
	return err
}

thread := client.StartThread(codex.ThreadOptions{})
turn, err := thread.Run(ctx, "Diagnose the failing test.", codex.TurnOptions{})
if err != nil {
	return err
}
fmt.Println(turn.FinalResponse)
```

`NewClient` resolves `codex` from `PATH`. Set `ClientOptions.CodexPath` when the
executable is installed elsewhere. Resolution happens when the Client is
created, so an unavailable CLI is reported immediately as `ExecutableError`.

## Continue and resume Threads

A Thread is a persisted conversation. Run consecutive Turns on the same Thread
to retain context:

```go
first, err := thread.Run(ctx, "Diagnose the failure.", codex.TurnOptions{})
if err != nil {
	return err
}
second, err := thread.Run(ctx, "Implement the fix.", codex.TurnOptions{})
```

After the CLI emits `thread.started`, `ID` returns the identifier needed to
resume the Thread in another process:

```go
threadID, ok := thread.ID()
if !ok {
	return errors.New("the Thread has not started")
}
resumed, err := client.ResumeThread(threadID, codex.ThreadOptions{})
if err != nil {
	return err
}
turn, err := resumed.Run(ctx, "Continue the work.", codex.TurnOptions{})
```

## Stream Thread Events

`RunStreamed` returns a lazy, single-use `iter.Seq2`. The CLI starts on first
iteration. Breaking the loop synchronously stops and reaps the process and
removes temporary resources.

```go
for event, err := range thread.RunStreamed(
	ctx,
	"Implement the fix.",
	codex.TurnOptions{},
) {
	if err != nil {
		return err
	}
	switch event := event.(type) {
	case *codex.ItemStartedEvent:
		fmt.Printf("started %T\n", event.Item)
	case *codex.ItemCompletedEvent:
		fmt.Printf("completed %T\n", event.Item)
	case *codex.TurnCompletedEvent:
		fmt.Printf("output tokens: %d\n", event.Usage.OutputTokens)
	case *codex.UnknownEvent:
		log.Printf("unknown Thread Event %q", event.UnknownType)
	}
}
```

Unknown Thread Events and Thread Items preserve their complete raw JSON. Handle
them in strict type switches so a newer CLI can add variants without breaking
an otherwise valid Turn. Invalid payloads for known variants still return a
`DecodeError`.

Cancel the supplied context to stop a Turn or enforce a deadline:

```go
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()
turn, err := thread.Run(ctx, "Run the checks.", codex.TurnOptions{})
if errors.Is(err, context.DeadlineExceeded) {
	return err
}
```

A Client is safe for concurrent use. Different Threads may run concurrently,
but one Thread permits only one active Turn. A concurrent attempt returns
`ErrTurnInProgress`; the SDK never queues it implicitly.

## Images and structured output

Use `StructuredInput` to send prompt text and local images in one Turn:

```go
turn, err := thread.Run(ctx, codex.StructuredInput{
	Text:        "Compare these screenshots.",
	LocalImages: []string{"./before.png", "./after.png"},
}, codex.TurnOptions{})
```

Provide a top-level JSON Schema through `TurnOptions` when the raw JSON response
is useful:

```go
schema := json.RawMessage(`{
  "type": "object",
  "properties": {"summary": {"type": "string"}},
  "required": ["summary"],
  "additionalProperties": false
}`)
turn, err := thread.Run(ctx, "Summarize the repository.", codex.TurnOptions{
	OutputSchema: schema,
})
fmt.Println(turn.FinalResponse)
```

`RunJSON` uses the same Turn pipeline and strictly decodes the response into a
Go type:

```go
type Summary struct {
	Summary string `json:"summary"`
}

result, err := thread.RunJSON[Summary](
	ctx,
	"Summarize the repository.",
	schema,
	codex.TurnOptions{},
)
fmt.Println(result.Output.Summary)
```

The SDK writes output schemas to private temporary files and removes them after
success, failure, cancellation, or early streaming termination.

## Configure CLI execution

Client options apply to every Thread:

```go
client, err := codex.NewClient(codex.ClientOptions{
	CodexPath: "/opt/codex/bin/codex",
	BaseURL:   "https://api.example.test/v1",
	APIKey:    os.Getenv("CODEX_API_KEY"),
	Config: map[string]any{
		"features": map[string]any{"plugins": false},
	},
	ConfigOverrides: []string{`model_reasoning_effort="high"`},
})
```

Thread options configure model selection, reasoning effort, sandboxing,
approval policy, working directories, network access, and web search:

```go
thread := client.StartThread(codex.ThreadOptions{
	Model:                 "gpt-5.6-sol",
	WorkingDirectory:      repository,
	AdditionalDirectories: []string{"../shared"},
	SandboxMode:           codex.SandboxWorkspaceWrite,
	ModelReasoningEffort:  codex.ReasoningEffortHigh,
	NetworkAccess:         codex.NetworkAccessDisabled,
	WebSearchMode:         codex.WebSearchCached,
	ApprovalPolicy:        codex.ApprovalOnRequest,
})
```

CLI configuration precedence is:

1. Flattened structured `Config`, rendered as TOML-compatible values
2. Ordered raw `ConfigOverrides`
3. SDK-managed settings such as `BaseURL`
4. Thread-specific settings

Structured configuration is validated recursively when the Client is created.
Invalid values return a path-aware `ValidationError`.

`ClientOptions.Env == nil` snapshots the host environment. A non-nil map,
including an empty map, replaces the child environment completely. The SDK then
sets its originator and, when configured, `CODEX_API_KEY`. Options, maps, and
slices are snapshotted so later caller mutation cannot alter execution.

## Errors

Use `errors.Is` for `ErrTurnInProgress`, `ErrStreamConsumed`, and context
cancellation. Use `errors.As` for diagnostic types including `ValidationError`,
`ExecutableError`, `ExecError`, `DecodeError`, `ProtocolError`,
`TurnFailedError`, and `OutputDecodeError`. Process stderr and protocol previews
are bounded, and configured API keys are redacted from captured stderr.

## Application-owned test adapters

Go does not permit generic methods in interfaces. Keep the SDK transport private
and define a narrow interface at the application boundary instead:

```go
type Summarizer interface {
	Summarize(context.Context, string) (string, error)
}

type codexSummarizer struct {
	thread *codex.Thread
}

func (s codexSummarizer) Summarize(ctx context.Context, input string) (string, error) {
	turn, err := s.thread.Run(ctx, input, codex.TurnOptions{})
	return turn.FinalResponse, err
}
```

Application tests can replace `Summarizer` without mocking processes, JSONL, or
temporary schema files.

## Development

From `sdk/go`, run:

```console
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
```

The fake CLI test fixture is a Go program and works on Linux, macOS, and Windows.
CI also runs a Linux integration test against the repository-built Codex CLI and
a local mock Responses API server.
