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
