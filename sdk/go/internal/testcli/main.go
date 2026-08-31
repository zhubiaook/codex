// Command testcli is a cross-platform fake Codex executable used by SDK tests.
package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
)

func main() {
	if !slices.Equal(os.Args[1:], []string{"exec", "--experimental-json"}) {
		fmt.Fprintf(os.Stderr, "unexpected arguments: %q", os.Args[1:])
		os.Exit(2)
	}
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read stdin: %v", err)
		os.Exit(2)
	}
	if string(prompt) != os.Getenv("EXPECTED_PROMPT") {
		fmt.Fprintf(os.Stderr, "unexpected prompt: %q", prompt)
		os.Exit(2)
	}

	switch os.Getenv("CODEX_FAKE_SCENARIO") {
	case "success":
		fmt.Println(`{"type":"thread.started","thread_id":"thread-1"}`)
		fmt.Println(`{"type":"turn.started"}`)
		fmt.Println(`{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"The test fails in parser.go."}}`)
		fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":42,"cached_input_tokens":12,"cache_write_input_tokens":3,"output_tokens":8,"reasoning_output_tokens":2}}`)
	case "exit":
		fmt.Fprint(os.Stderr, strings.Repeat("failure", 20_000))
		os.Exit(7)
	case "malformed":
		fmt.Println(strings.Repeat("{", 5_000))
	case "missing-terminal":
		fmt.Println(`{"type":"thread.started","thread_id":"thread-1"}`)
		fmt.Println(`{"type":"turn.started"}`)
		fmt.Println(`{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"partial"}}`)
	default:
		fmt.Fprintf(os.Stderr, "unknown scenario: %q", os.Getenv("CODEX_FAKE_SCENARIO"))
		os.Exit(2)
	}
}
