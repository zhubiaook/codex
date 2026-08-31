// Command testcli is a cross-platform fake Codex executable used by SDK tests.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

func main() {
	prompt, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read stdin: %v", err)
		os.Exit(2)
	}
	if expected := os.Getenv("EXPECTED_PROMPT"); expected != "" && string(prompt) != expected {
		fmt.Fprintf(os.Stderr, "unexpected prompt: %q", prompt)
		os.Exit(2)
	}

	switch os.Getenv("CODEX_FAKE_SCENARIO") {
	case "success":
		requireArgs("exec", "--experimental-json")
		fmt.Println(`{"type":"thread.started","thread_id":"thread-1"}`)
		fmt.Println(`{"type":"turn.started"}`)
		fmt.Println(`{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"The test fails in parser.go."}}`)
		fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":42,"cached_input_tokens":12,"cache_write_input_tokens":3,"output_tokens":8,"reasoning_output_tokens":2}}`)
	case "exit":
		requireArgs("exec", "--experimental-json")
		fmt.Fprint(os.Stderr, strings.Repeat("failure", 20_000))
		os.Exit(7)
	case "malformed":
		requireArgs("exec", "--experimental-json")
		fmt.Println(strings.Repeat("{", 5_000))
	case "missing-terminal":
		requireArgs("exec", "--experimental-json")
		fmt.Println(`{"type":"thread.started","thread_id":"thread-1"}`)
		fmt.Println(`{"type":"turn.started"}`)
		fmt.Println(`{"type":"item.completed","item":{"id":"item-1","type":"agent_message","text":"partial"}}`)
	case "sequence":
		runSequence(string(prompt))
	case "resume":
		requireArgs("exec", "--experimental-json", "resume", "persisted-thread")
		if string(prompt) != "resume" {
			fail("unexpected resume prompt: %q", prompt)
		}
		emitTurn("persisted-thread", "resumed response")
	case "stream":
		requireArgs("exec", "--experimental-json")
		if err := os.WriteFile(os.Getenv("CODEX_FAKE_STATE"), []byte("started"), 0o600); err != nil {
			fail("write stream state: %v", err)
		}
		emitTurn("thread-stream", "stream response")
	case "early-break":
		runEarlyBreak(string(prompt))
	case "cancel":
		requireArgs("exec", "--experimental-json")
		fmt.Println(`{"type":"thread.started","thread_id":"thread-cancel"}`)
		time.Sleep(30 * time.Second)
	default:
		fmt.Fprintf(os.Stderr, "unknown scenario: %q", os.Getenv("CODEX_FAKE_SCENARIO"))
		os.Exit(2)
	}
}

func runEarlyBreak(prompt string) {
	statePath := os.Getenv("CODEX_FAKE_STATE")
	_, err := os.Stat(statePath)
	if errors.Is(err, os.ErrNotExist) {
		requireArgs("exec", "--experimental-json")
		if prompt != "first" {
			fail("unexpected first early-break prompt: %q", prompt)
		}
		if err := os.WriteFile(statePath, []byte("started"), 0o600); err != nil {
			fail("write early-break state: %v", err)
		}
		fmt.Println(`{"type":"thread.started","thread_id":"thread-early"}`)
		time.Sleep(30 * time.Second)
		return
	}
	if err != nil {
		fail("read early-break state: %v", err)
	}
	requireArgs("exec", "--experimental-json", "resume", "thread-early")
	if prompt != "second" {
		fail("unexpected second early-break prompt: %q", prompt)
	}
	emitTurn("thread-early", "after early break")
}

func runSequence(prompt string) {
	statePath := os.Getenv("CODEX_FAKE_STATE")
	_, err := os.Stat(statePath)
	if errors.Is(err, os.ErrNotExist) {
		requireArgs("exec", "--experimental-json", "--thread-source", "automated_review")
		if prompt != "first" {
			fail("unexpected first prompt: %q", prompt)
		}
		if err := os.WriteFile(statePath, []byte("started"), 0o600); err != nil {
			fail("write sequence state: %v", err)
		}
		emitTurn("thread-sequence", "first response")
		return
	}
	if err != nil {
		fail("read sequence state: %v", err)
	}
	requireArgs("exec", "--experimental-json", "resume", "thread-sequence")
	if prompt != "second" {
		fail("unexpected second prompt: %q", prompt)
	}
	emitTurn("thread-sequence", "second response")
}

func emitTurn(threadID string, response string) {
	fmt.Printf("{\"type\":\"thread.started\",\"thread_id\":%q}\n", threadID)
	fmt.Println(`{"type":"turn.started"}`)
	fmt.Printf("{\"type\":\"item.completed\",\"item\":{\"id\":\"item-1\",\"type\":\"agent_message\",\"text\":%q}}\n", response)
	fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":1,"cached_input_tokens":0,"output_tokens":1,"reasoning_output_tokens":0}}`)
}

func requireArgs(expected ...string) {
	if !slices.Equal(os.Args[1:], expected) {
		fail("unexpected arguments: %q, want %q", os.Args[1:], expected)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(2)
}
