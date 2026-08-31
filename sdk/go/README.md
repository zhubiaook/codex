# Codex Go SDK

Embed the Codex agent in Go applications and workflows.

The Go SDK wraps an installed `codex` CLI, sends Turn input through stdin, and
reads JSONL events from stdout. It requires Go 1.27 or newer.

## Installation

Install the Codex CLI and make sure `codex` is available on `PATH`, then add the
SDK module:

```console
go get github.com/zhubiaook/codex/sdk/go
```

## Quickstart

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/zhubiaook/codex/sdk/go"
)

func main() {
	client, err := codex.NewClient(codex.ClientOptions{})
	if err != nil {
		log.Fatal(err)
	}

	thread := client.StartThread(codex.ThreadOptions{})
	turn, err := thread.Run(
		context.Background(),
		"Diagnose the failing test.",
		codex.TurnOptions{},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(turn.FinalResponse)
}
```

Set `ClientOptions.CodexPath` to use a specific Codex CLI executable. Set
`ClientOptions.Env` to replace the child process environment; leave it nil to
snapshot the current process environment when the Client is created.

Call `Run` repeatedly on the same Thread to continue the conversation:

```go
first, err := thread.Run(ctx, "Diagnose the failure.", codex.TurnOptions{})
if err != nil {
	return err
}
second, err := thread.Run(ctx, "Implement the fix.", codex.TurnOptions{})
```

Threads are persisted by the Codex CLI. Save the identifier and reconstruct the
Thread in another process when needed:

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

## Streaming Thread Events

Use `RunStreamed` to process Thread Events as the Codex CLI emits them:

```go
events := thread.RunStreamed(ctx, "Implement the fix.", codex.TurnOptions{})
for event, err := range events {
	if err != nil {
		return err
	}
	switch event := event.(type) {
	case *codex.ItemCompletedEvent:
		fmt.Printf("completed %T\n", event.Item)
	case *codex.TurnCompletedEvent:
		fmt.Printf("output tokens: %d\n", event.Usage.OutputTokens)
	}
}
```

The iterator is lazy and single-use: the CLI starts on the first iteration, not
when `RunStreamed` returns. Breaking out of the loop synchronously stops and
reaps the CLI process. Cancel the supplied context to stop a Turn from another
goroutine or enforce a deadline.

A Thread permits one active Turn and returns `ErrTurnInProgress` for a
concurrent attempt. Separate Threads from the same Client can run concurrently.
