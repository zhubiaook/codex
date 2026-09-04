# Codex Go SDK

Use Codex from Go applications and automation. The SDK starts an installed
`codex` CLI process, sends each Turn through standard input, and decodes the
CLI's JSONL output into typed Thread Events and Thread Items.

The SDK supports the OpenAI API and third-party providers that implement the
OpenAI **Responses API**. Chat Completions-only endpoints are not compatible.

## Requirements

- Go 1.27 or newer
- A compatible `codex` CLI on `PATH`, or its path supplied through
  `ClientOptions.CodexPath`
- API credentials for the selected model provider

Verify the tools before creating a project:

```console
go version
codex --version
```

## Install

From an existing Go module:

```console
go get github.com/zhubiaook/codex/sdk/go
```

Tags for this nested module use the `sdk/go/vX.Y.Z` convention.

## Quickstart

Set an API key:

```console
export CODEX_API_KEY="your-api-key"
```

Create `main.go`:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	codex "github.com/zhubiaook/codex/sdk/go"
)

func main() {
	client, err := codex.NewClient(codex.ClientOptions{
		APIKey: os.Getenv("CODEX_API_KEY"),
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	thread := client.StartThread(codex.ThreadOptions{
		WorkingDirectory: ".",
	})
	turn, err := thread.Run(ctx, "Summarize this repository.", codex.TurnOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(turn.FinalResponse)
}
```

Run it from a Git repository:

```console
go run .
```

Codex normally requires the working directory to be a Git repository. Set
`SkipGitRepoCheck: true` only when the application intentionally operates in a
non-repository directory.

`NewClient` resolves `codex` when the Client is created. To use a specific
binary, set `CodexPath` to its absolute path, such as
`/opt/codex/bin/codex`.

## Configure a third-party LLM

The provider must implement the OpenAI Responses API, including streamed
Responses events. A base URL identifies the API root immediately before the
final `/responses` route. For example, use `https://provider.example/v1`, not
`https://provider.example/v1/responses`.

The SDK does not load `.env` files. Export their values before starting the Go
program, or load them with the configuration library used by your application.
For a shell-compatible `.env` file:

```console
set -a
. ./.env
set +a
```

The following complete program accepts any Responses-compatible provider
through `LLM_API_KEY`, `LLM_BASE_URL`, and `LLM_MODEL`. It also tolerates a
`LLM_BASE_URL` that already ends in `/responses` by removing that final route.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	codex "github.com/zhubiaook/codex/sdk/go"
)

func main() {
	apiKey := requiredEnv("LLM_API_KEY")
	baseURL := responsesBaseURL(requiredEnv("LLM_BASE_URL"))
	model := requiredEnv("LLM_MODEL")

	const providerID = "third_party"
	client, err := codex.NewClient(codex.ClientOptions{
		APIKey: apiKey,
		Config: map[string]any{
			"model_provider": providerID,
			"model_providers": map[string]any{
				providerID: map[string]any{
					"name":                "Third-party Responses API",
					"base_url":            baseURL,
					"env_key":             "CODEX_API_KEY",
					"wire_api":            "responses",
					"supports_websockets": false,
				},
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	thread := client.StartThread(codex.ThreadOptions{
		Model:            model,
		WorkingDirectory: ".",
		SandboxMode:      codex.SandboxReadOnly,
		WebSearchMode:    codex.WebSearchDisabled,
		ApprovalPolicy:   codex.ApprovalNever,
	})
	turn, err := thread.Run(ctx, "Reply with: provider connected", codex.TurnOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(turn.FinalResponse)
}

func requiredEnv(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		log.Fatalf("%s is not set", name)
	}
	return value
}

func responsesBaseURL(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	value, _ = strings.CutSuffix(value, "/responses")
	if value == "" {
		log.Fatal("LLM_BASE_URL does not contain an API base URL")
	}
	return value
}
```

For a `.env` file that uses provider-specific names, map them when starting the
program. For example:

```console
set -a
. ./.env
set +a
LLM_API_KEY="$DASHSCOPE_API_KEY" \
LLM_BASE_URL="$DASHSCOPE_BASE_URL" \
LLM_MODEL="$DASHSCOPE_MODEL" \
go run .
```

Why the custom provider configuration matters:

- `APIKey` is injected into the child process as `CODEX_API_KEY`; `env_key`
  tells the provider which variable to read.
- `wire_api: "responses"` selects the Responses protocol explicitly.
- `supports_websockets: false` prevents retries against providers that only
  support HTTP streaming.
- `Model` is set per Thread, so the same Client configuration can create
  Threads with different models supported by the provider.

`ClientOptions.BaseURL` is a shorter way to override the built-in OpenAI
provider's base URL. Prefer an explicit `model_provider` definition for a
third-party service because transport and authentication behavior are then
unambiguous.

## Continue and resume Threads

A Thread is a persisted conversation. Calling `Run` again on the same Thread
continues it. After the CLI emits `thread.started`, `ID` returns the identifier
needed to reconstruct that Thread later.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	codex "github.com/zhubiaook/codex/sdk/go"
)

func main() {
	client, err := codex.NewClient(codex.ClientOptions{
		APIKey: os.Getenv("CODEX_API_KEY"),
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	thread := client.StartThread(codex.ThreadOptions{WorkingDirectory: "."})
	first, err := thread.Run(ctx, "Find the most important package in this repository.", codex.TurnOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(first.FinalResponse)

	second, err := thread.Run(ctx, "Explain why you selected it.", codex.TurnOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(second.FinalResponse)

	threadID, ok := thread.ID()
	if !ok {
		log.Fatal(errors.New("the Thread has not started"))
	}
	resumed, err := client.ResumeThread(threadID, codex.ThreadOptions{WorkingDirectory: "."})
	if err != nil {
		log.Fatal(err)
	}
	turn, err := resumed.Run(ctx, "Give the explanation in one sentence.", codex.TurnOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(turn.FinalResponse)
}
```

Thread records are stored by the Codex CLI under `CODEX_HOME` (normally
`~/.codex`). A different process can call `ResumeThread` when it uses the same
Codex home and Thread ID.

## Stream Thread Events

`RunStreamed` returns a lazy, single-use `iter.Seq2`. The CLI starts when
iteration begins. Consumer iteration provides backpressure; breaking the loop
stops and reaps the CLI process and removes temporary schema files.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	codex "github.com/zhubiaook/codex/sdk/go"
)

func main() {
	client, err := codex.NewClient(codex.ClientOptions{
		APIKey: os.Getenv("CODEX_API_KEY"),
	})
	if err != nil {
		log.Fatal(err)
	}
	thread := client.StartThread(codex.ThreadOptions{WorkingDirectory: "."})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for event, err := range thread.RunStreamed(
		ctx,
		"Run the relevant checks and report the result.",
		codex.TurnOptions{},
	) {
		if err != nil {
			log.Fatal(err)
		}
		switch event := event.(type) {
		case *codex.ThreadStartedEvent:
			fmt.Printf("thread: %s\n", event.ThreadID)
		case *codex.ItemStartedEvent:
			fmt.Printf("started: %T\n", event.Item)
		case *codex.ItemUpdatedEvent:
			fmt.Printf("updated: %T\n", event.Item)
		case *codex.ItemCompletedEvent:
			fmt.Printf("completed: %T\n", event.Item)
		case *codex.TurnCompletedEvent:
			fmt.Printf("output tokens: %d\n", event.Usage.OutputTokens)
		case *codex.TurnFailedEvent:
			log.Fatalf("Turn failed: %s", event.Error.Message)
		case *codex.ThreadErrorEvent:
			log.Fatal(event)
		case *codex.UnknownEvent:
			log.Printf("unknown Thread Event %q", event.UnknownType)
		}
	}
}
```

Unknown Thread Events and Thread Items retain an owned copy of the complete raw
JSON. Invalid payloads for known variants return `DecodeError` instead of being
downgraded to unknown values.

One Thread permits one active Turn. Concurrent work is supported by creating
separate Threads from the same Client. Reusing an active Thread returns
`ErrTurnInProgress`; iterating the same stream twice returns `ErrStreamConsumed`.

## Images

Use `StructuredInput` to combine prompt text with local image paths. The chosen
model and provider must support image input.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	codex "github.com/zhubiaook/codex/sdk/go"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("usage: %s IMAGE_PATH", os.Args[0])
	}
	client, err := codex.NewClient(codex.ClientOptions{
		APIKey: os.Getenv("CODEX_API_KEY"),
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	turn, err := client.StartThread(codex.ThreadOptions{WorkingDirectory: "."}).Run(
		ctx,
		codex.StructuredInput{
			Text:        "Describe this image.",
			LocalImages: []string{os.Args[1]},
		},
		codex.TurnOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(turn.FinalResponse)
}
```

Run the program with a local file:

```console
go run . ./screenshot.png
```

## Structured output

`TurnOptions.OutputSchema` returns the model response as a string while asking
the provider to conform to a JSON Schema. `RunJSON` uses the same Turn pipeline
and also decodes that response into a Go type. The provider and model must
support structured Responses output.

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	codex "github.com/zhubiaook/codex/sdk/go"
)

type summary struct {
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

func main() {
	client, err := codex.NewClient(codex.ClientOptions{
		APIKey: os.Getenv("CODEX_API_KEY"),
	})
	if err != nil {
		log.Fatal(err)
	}

	schema := json.RawMessage(`{
  "type": "object",
  "properties": {
    "summary": {"type": "string"},
    "status": {"type": "string", "enum": ["ok", "action_required"]}
  },
  "required": ["summary", "status"],
  "additionalProperties": false
}`)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := client.StartThread(codex.ThreadOptions{WorkingDirectory: "."}).RunJSON[summary](
		ctx,
		"Summarize the repository status. Return only the requested JSON object without Markdown fences.",
		schema,
		codex.TurnOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: %s\n", result.Output.Status, result.Output.Summary)
}
```

Schemas must be valid top-level JSON objects. The SDK writes each schema to a
private temporary file for the CLI and removes it after success, failure,
cancellation, or early streaming termination. `RunJSON` rejects invalid JSON,
target type mismatches, and fields not represented by the destination type.

## Configuration reference

### Client options

| Field | Purpose |
| --- | --- |
| `CodexPath` | Explicit path to the Codex CLI; otherwise resolve `codex` from `PATH`. |
| `BaseURL` | Override the built-in OpenAI provider's API base URL. Do not include the final `/responses`. |
| `APIKey` | Inject an API key into the CLI as `CODEX_API_KEY`. |
| `Config` | Structured, recursively validated Codex configuration rendered as TOML-compatible overrides. |
| `ConfigOverrides` | Ordered raw `key=value` CLI overrides. |
| `Env` | Exact base environment for the child process; `nil` snapshots the host environment. |

### Thread options

| Field | Purpose |
| --- | --- |
| `Model` | Model identifier sent to the provider. |
| `ThreadSource` | Classification for a new Thread; omitted when resuming. |
| `SandboxMode` | `SandboxReadOnly`, `SandboxWorkspaceWrite`, or `SandboxDangerFullAccess`. |
| `WorkingDirectory` | Working directory visible to the agent. |
| `AdditionalDirectories` | Additional directories granted to the agent. |
| `SkipGitRepoCheck` | Permit a working directory that is not a Git repository. |
| `ModelReasoningEffort` | Requested reasoning effort, when supported by the model. |
| `NetworkAccess` | Explicitly enable or disable workspace network access. |
| `WebSearchMode` | Disable, cache, or enable live web search. |
| `ApprovalPolicy` | Control when Codex requests approval. |

Configuration overrides are applied in this order, with later values taking
precedence:

1. Flattened structured `Config`
2. Ordered raw `ConfigOverrides`
3. SDK-managed settings such as `BaseURL`
4. Thread-specific settings

`ClientOptions.Env == nil` snapshots the process environment when the Client is
created. A non-nil map, including an empty map, replaces the inherited
environment before SDK-managed variables are injected. Client options and
Thread slices are snapshotted, so later caller mutation cannot change execution.

## Cancellation and errors

Every Turn accepts a `context.Context`. Cancel it directly or use a deadline to
stop and reap the CLI process. Context errors remain detectable with
`errors.Is`.

Use `errors.Is` for:

- `ErrTurnInProgress`
- `ErrStreamConsumed`
- `context.Canceled`
- `context.DeadlineExceeded`

Use `errors.As` or Go 1.27's `errors.AsType` for:

- `ValidationError`: invalid SDK input or configuration
- `ExecutableError`: the Codex CLI could not be resolved
- `ExecError`: the CLI could not start or exited unsuccessfully
- `DecodeError`: malformed or invalid CLI JSONL
- `ProtocolError`: an invalid event sequence
- `TurnFailedError`: a `turn.failed` event
- `OutputDecodeError`: structured output could not be decoded into the target type

Captured stderr and protocol previews are bounded. An API key supplied through
`ClientOptions.APIKey` is redacted from captured stderr.

## Testing applications that use the SDK

Go generic methods cannot be interface methods. Define a narrow,
application-owned interface around the operation your application needs, then
replace that adapter in application tests. This keeps tests independent of CLI
processes, JSONL, and temporary schema files.

## Troubleshooting third-party providers

### The request path ends in `/responses/responses`

The configured base URL already includes the final route. Remove the trailing
`/responses`; the provider appends it when sending a request.

### WebSocket connections retry before HTTP succeeds

Set `supports_websockets` to `false` in the custom provider configuration.

### The provider returns 404 or 405

Confirm that the service implements `POST /responses`. An endpoint that only
implements `POST /chat/completions` is not compatible with this SDK transport.

### `RunJSON` reports an invalid character such as a backtick

The model returned Markdown fences instead of a raw JSON object. Confirm that
the provider supports structured Responses output and explicitly ask the model
to return the JSON object without Markdown. Do not strip fences in application
code: doing so can hide a provider that ignored the requested schema.

### The model is unknown to Codex

Codex may use fallback model metadata for third-party model identifiers. Basic
text requests can still work, but unsupported reasoning, image, or structured
output capabilities depend on the provider and selected model.

## SDK development

From `sdk/go`, run:

```console
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
```

The test suite builds a cross-platform fake Codex executable. CI runs the SDK
checks on Linux, macOS, and Windows and also runs a Linux integration test
against the repository-built Codex CLI and a local mock Responses API server.
