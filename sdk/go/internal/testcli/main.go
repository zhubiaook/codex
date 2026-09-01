// Command testcli is a cross-platform fake Codex executable used by SDK tests.
package main

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	case "secret-exit":
		requireArgs("exec", "--experimental-json")
		fmt.Fprintf(os.Stderr, "credential=%s", os.Getenv("CODEX_API_KEY"))
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
	case "backpressure":
		runBackpressure()
	case "early-break":
		runEarlyBreak(string(prompt))
	case "cancel":
		requireArgs("exec", "--experimental-json")
		fmt.Println(`{"type":"thread.started","thread_id":"thread-cancel"}`)
		time.Sleep(30 * time.Second)
	case "args":
		requireExpectedArgs()
		requireEnvironment("CODEX_API_KEY", "EXPECTED_API_KEY")
		requireEnvironment("CODEX_INTERNAL_ORIGINATOR_OVERRIDE", "EXPECTED_ORIGINATOR")
		if key := os.Getenv("EXPECTED_ABSENT"); key != "" && os.Getenv(key) != "" {
			fail("environment %s must be absent", key)
		}
		emitTurn("thread-args", "configured")
	case "structured", "structured-exit", "structured-cancel", "structured-early-break":
		runStructured(os.Getenv("CODEX_FAKE_SCENARIO"))
	case "activity":
		emitActivity()
	case "malformed-known":
		fmt.Println(`{"type":"item.completed","item":{"id":"command-1","type":"command_execution","command":"go test","aggregated_output":"","status":"invented"}}`)
	case "malformed-completion":
		fmt.Println(`{"type":"turn.completed"}`)
	case "turn-failed":
		fmt.Println(`{"type":"turn.failed","error":{"message":"model failed"}}`)
	case "thread-error":
		fmt.Println(`{"type":"error","message":"stream failed"}`)
	case "integration-items":
		emitIntegrationItems()
	default:
		fmt.Fprintf(os.Stderr, "unknown scenario: %q", os.Getenv("CODEX_FAKE_SCENARIO"))
		os.Exit(2)
	}
}

func runBackpressure() {
	requireArgs("exec", "--experimental-json")
	statePath := os.Getenv("CODEX_FAKE_STATE")
	fmt.Println(`{"type":"thread.started","thread_id":"thread-backpressure"}`)
	if err := os.WriteFile(statePath, []byte("writing"), 0o600); err != nil {
		fail("write backpressure state: %v", err)
	}
	fmt.Printf(`{"type":"future.event","payload":"%s"}`+"\n", strings.Repeat("x", 2<<20))
	if err := os.WriteFile(statePath, []byte("written"), 0o600); err != nil {
		fail("advance backpressure state: %v", err)
	}
	fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":1,"cached_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0}}`)
}

func emitActivity() {
	fmt.Println(`{"type":"thread.started","thread_id":"thread-activity"}`)
	fmt.Println(`{"type":"turn.started"}`)
	fmt.Println(`{"type":"item.started","item":{"id":"command-1","type":"command_execution","command":"go test ./...","aggregated_output":"","status":"in_progress"}}`)
	fmt.Println(`{"type":"item.updated","item":{"id":"command-1","type":"command_execution","command":"go test ./...","aggregated_output":"ok\n","status":"in_progress"}}`)
	fmt.Println(`{"type":"item.completed","item":{"id":"command-1","type":"command_execution","command":"go test ./...","aggregated_output":"ok\n","exit_code":0,"status":"completed"}}`)
	fmt.Println(`{"type":"item.completed","item":{"id":"reasoning-1","type":"reasoning","text":"Inspect the failure."}}`)
	fmt.Println(`{"type":"item.completed","item":{"id":"patch-1","type":"file_change","changes":[{"path":"new.go","kind":"add"},{"path":"old.go","kind":"delete"},{"path":"changed.go","kind":"update"}],"status":"completed"}}`)
	fmt.Println(`{"type":"item.completed","item":{"id":"message-1","type":"agent_message","text":"Done."}}`)
	fmt.Println(`{"type":"future.event","answer":42}`)
	fmt.Println(`{"type":"item.completed","item":{"id":"future-1","type":"future_item","payload":{"ok":true}}}`)
	fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":4,"cached_input_tokens":2,"output_tokens":3,"reasoning_output_tokens":1}}`)
}

func emitIntegrationItems() {
	fmt.Println(`{"type":"thread.started","thread_id":"thread-integrations"}`)
	fmt.Println(`{"type":"turn.started"}`)
	fmt.Println(`{"type":"item.started","item":{"id":"mcp-start","type":"mcp_tool_call","server":"files","tool":"read","arguments":{"path":"README.md"},"status":"in_progress"}}`)
	fmt.Println(`{"type":"item.updated","item":{"id":"mcp-start","type":"mcp_tool_call","server":"files","tool":"read","arguments":{"path":"README.md"},"result":{"content":[],"structured_content":null},"status":"in_progress"}}`)
	fmt.Println(`{"type":"item.completed","item":{"id":"mcp-success","type":"mcp_tool_call","server":"db","tool":"query","arguments":{"sql":"select 1"},"result":{"content":[{"type":"text","text":"1"}],"_meta":{"trace":"abc"},"structured_content":{"rows":[1]}},"status":"completed"}}`)
	fmt.Println(`{"type":"item.completed","item":{"id":"mcp-failure","type":"mcp_tool_call","server":"db","tool":"query","arguments":null,"error":{"message":"permission denied"},"status":"failed"}}`)
	fmt.Println(`{"type":"item.completed","item":{"id":"search-1","type":"web_search","query":"Go 1.27 release notes"}}`)
	fmt.Println(`{"type":"item.completed","item":{"id":"todo-1","type":"todo_list","items":[{"text":"Inspect","completed":true},{"text":"Implement","completed":false}]}}`)
	fmt.Println(`{"type":"item.completed","item":{"id":"error-1","type":"error","message":"non-fatal warning"}}`)
	fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":1,"cached_input_tokens":0,"output_tokens":1,"reasoning_output_tokens":0}}`)
}

func runStructured(scenario string) {
	args := slices.Clone(os.Args[1:])
	schemaIndex := slices.Index(args, "--output-schema")
	if schemaIndex < 0 || schemaIndex+1 >= len(args) {
		fail("missing --output-schema argument: %q", args)
	}
	schemaPath := args[schemaIndex+1]
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		fail("read output schema: %v", err)
	}
	if string(schema) != os.Getenv("EXPECTED_SCHEMA") {
		fail("output schema = %q, want %q", schema, os.Getenv("EXPECTED_SCHEMA"))
	}
	if info, err := os.Stat(schemaPath); err != nil {
		fail("stat output schema: %v", err)
	} else if info.Mode().Perm()&0o077 != 0 && os.PathSeparator != '\\' {
		fail("output schema permissions = %o, want private", info.Mode().Perm())
	}
	if statePath := os.Getenv("CODEX_FAKE_STATE"); statePath != "" {
		if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
			fail("create schema state directory: %v", err)
		}
		if err := os.WriteFile(statePath, []byte(schemaPath), 0o600); err != nil {
			fail("record schema path: %v", err)
		}
	}

	var expectedImages []string
	if err := json.Unmarshal([]byte(os.Getenv("EXPECTED_IMAGES")), &expectedImages); err != nil {
		fail("decode EXPECTED_IMAGES: %v", err)
	}
	withoutSchema := append(slices.Clone(args[:schemaIndex]), args[schemaIndex+2:]...)
	expected := []string{"exec", "--experimental-json"}
	if resumeID := os.Getenv("EXPECTED_RESUME_ID"); resumeID != "" {
		expected = append(expected, "resume", resumeID)
	}
	for _, image := range expectedImages {
		expected = append(expected, "--image", image)
	}
	if !slices.Equal(withoutSchema, expected) {
		fail("unexpected structured arguments: %q, want %q", withoutSchema, expected)
	}

	switch scenario {
	case "structured":
		emitTurn("thread-structured", os.Getenv("STRUCTURED_RESPONSE"))
	case "structured-exit":
		fmt.Fprint(os.Stderr, "structured failure")
		os.Exit(7)
	case "structured-cancel", "structured-early-break":
		fmt.Println(`{"type":"thread.started","thread_id":"thread-structured"}`)
		time.Sleep(30 * time.Second)
	}
}

func requireExpectedArgs() {
	var expected []string
	if err := json.Unmarshal([]byte(os.Getenv("EXPECTED_ARGS")), &expected); err != nil {
		fail("decode EXPECTED_ARGS: %v", err)
	}
	requireArgs(expected...)
}

func requireEnvironment(key string, expectationKey string) {
	if got, want := os.Getenv(key), os.Getenv(expectationKey); got != want {
		fail("environment %s = %q, want %q", key, got, want)
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
