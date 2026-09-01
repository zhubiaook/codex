package codex_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/zhubiaook/codex/sdk/go"
)

func TestClientRunsTextTurn(t *testing.T) {
	executable := buildFakeCodex(t)
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: executable,
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "success",
			"EXPECTED_PROMPT":     "Diagnose the failing test.",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	thread := client.StartThread(codex.ThreadOptions{})
	got, err := thread.Run(t.Context(), "Diagnose the failing test.", codex.TurnOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := codex.Turn{
		Items: []codex.ThreadItem{
			&codex.AgentMessageItem{ID: "item-1", Text: "The test fails in parser.go."},
		},
		FinalResponse: "The test fails in parser.go.",
		Usage: &codex.Usage{
			InputTokens:           42,
			CachedInputTokens:     12,
			CacheWriteInputTokens: 3,
			OutputTokens:          8,
			ReasoningOutputTokens: 2,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Run() = %#v, want %#v", got, want)
	}
}

func TestClientReturnsBoundedProcessError(t *testing.T) {
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "exit",
			"EXPECTED_PROMPT":     "fail",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.StartThread(codex.ThreadOptions{}).Run(
		t.Context(),
		"fail",
		codex.TurnOptions{},
	)
	execError, ok := errors.AsType[*codex.ExecError](err)
	if !ok {
		t.Fatalf("Run() error = %T %v, want *codex.ExecError", err, err)
	}
	if execError.ExitCode != 7 {
		t.Errorf("ExecError.ExitCode = %d, want 7", execError.ExitCode)
	}
	if len(execError.Stderr) > (64<<10)+32 {
		t.Errorf("len(ExecError.Stderr) = %d, want bounded stderr", len(execError.Stderr))
	}
	if !strings.HasSuffix(execError.Stderr, "[stderr truncated]") {
		t.Errorf("ExecError.Stderr does not report truncation: %q", execError.Stderr)
	}
}

func TestClientReturnsDecodeErrorForMalformedJSONL(t *testing.T) {
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "malformed",
			"EXPECTED_PROMPT":     "decode",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.StartThread(codex.ThreadOptions{}).Run(
		t.Context(),
		"decode",
		codex.TurnOptions{},
	)
	decodeError, ok := errors.AsType[*codex.DecodeError](err)
	if !ok {
		t.Fatalf("Run() error = %T %v, want *codex.DecodeError", err, err)
	}
	if decodeError.Line != 1 {
		t.Errorf("DecodeError.Line = %d, want 1", decodeError.Line)
	}
	if len(decodeError.Preview) > 4<<10 {
		t.Errorf("len(DecodeError.Preview) = %d, want at most 4096", len(decodeError.Preview))
	}
}

func TestClientRejectsMissingTerminalEvent(t *testing.T) {
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "missing-terminal",
			"EXPECTED_PROMPT":     "partial",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.StartThread(codex.ThreadOptions{}).Run(
		t.Context(),
		"partial",
		codex.TurnOptions{},
	)
	protocolError, ok := errors.AsType[*codex.ProtocolError](err)
	if !ok {
		t.Fatalf("Run() error = %T %v, want *codex.ProtocolError", err, err)
	}
	if protocolError.Message != "process exited without turn.completed" {
		t.Errorf("ProtocolError.Message = %q", protocolError.Message)
	}
}

func TestNewClientRejectsMissingExecutable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-codex")
	_, err := codex.NewClient(codex.ClientOptions{CodexPath: missing})
	executableError, ok := errors.AsType[*codex.ExecutableError](err)
	if !ok {
		t.Fatalf("NewClient() error = %T %v, want *codex.ExecutableError", err, err)
	}
	if executableError.Path != missing {
		t.Errorf("ExecutableError.Path = %q, want %q", executableError.Path, missing)
	}
}

func TestClientSnapshotsEnvironment(t *testing.T) {
	environment := map[string]string{
		"CODEX_FAKE_SCENARIO": "success",
		"EXPECTED_PROMPT":     "original",
	}
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env:       environment,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	environment["CODEX_FAKE_SCENARIO"] = "exit"
	environment["EXPECTED_PROMPT"] = "mutated"

	turn, err := client.StartThread(codex.ThreadOptions{}).Run(
		t.Context(),
		"original",
		codex.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if turn.FinalResponse != "The test fails in parser.go." {
		t.Errorf("Turn.FinalResponse = %q", turn.FinalResponse)
	}
}

func TestThreadContinuesWithEstablishedID(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "sequence-state")
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "sequence",
			"CODEX_FAKE_STATE":    statePath,
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	thread := client.StartThread(codex.ThreadOptions{ThreadSource: "automated_review"})

	first, err := thread.Run(t.Context(), "first", codex.TurnOptions{})
	if err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if first.FinalResponse != "first response" {
		t.Errorf("first Turn.FinalResponse = %q", first.FinalResponse)
	}
	if id, ok := thread.ID(); !ok || id != "thread-sequence" {
		t.Errorf("Thread.ID() = %q, %t, want %q, true", id, ok, "thread-sequence")
	}

	second, err := thread.Run(t.Context(), "second", codex.TurnOptions{})
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if second.FinalResponse != "second response" {
		t.Errorf("second Turn.FinalResponse = %q", second.FinalResponse)
	}
}

func TestClientResumesPersistedThread(t *testing.T) {
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "resume",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	thread, err := client.ResumeThread(
		"persisted-thread",
		codex.ThreadOptions{ThreadSource: "must-not-be-sent"},
	)
	if err != nil {
		t.Fatalf("ResumeThread() error = %v", err)
	}
	if id, ok := thread.ID(); !ok || id != "persisted-thread" {
		t.Errorf("Thread.ID() = %q, %t, want %q, true", id, ok, "persisted-thread")
	}

	turn, err := thread.Run(t.Context(), "resume", codex.TurnOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if turn.FinalResponse != "resumed response" {
		t.Errorf("Turn.FinalResponse = %q", turn.FinalResponse)
	}
}

func TestClientRejectsEmptyResumeID(t *testing.T) {
	client, err := codex.NewClient(codex.ClientOptions{CodexPath: buildFakeCodex(t)})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.ResumeThread("", codex.ThreadOptions{})
	validationError, ok := errors.AsType[*codex.ValidationError](err)
	if !ok {
		t.Fatalf("ResumeThread() error = %T %v, want *codex.ValidationError", err, err)
	}
	if validationError.Field != "id" {
		t.Errorf("ValidationError.Field = %q, want id", validationError.Field)
	}
}

func TestThreadIDIsSafeForConcurrentReaders(t *testing.T) {
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "success",
			"EXPECTED_PROMPT":     "concurrent",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	thread := client.StartThread(codex.ThreadOptions{})

	unexpectedIDs := make(chan string, 16)
	var readers sync.WaitGroup
	for range 16 {
		readers.Go(func() {
			for range 1_000 {
				if id, ok := thread.ID(); ok && id != "thread-1" {
					unexpectedIDs <- id
					return
				}
			}
		})
	}
	if _, err := thread.Run(t.Context(), "concurrent", codex.TurnOptions{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	readers.Wait()
	close(unexpectedIDs)
	for id := range unexpectedIDs {
		t.Errorf("Thread.ID() returned unexpected ID %q", id)
	}
}

func buildFakeCodex(t *testing.T) string {
	t.Helper()
	name := "fake-codex"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	directory := filepath.Join(t.TempDir(), "path with spaces")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatalf("create fake Codex directory: %v", err)
	}
	executable := filepath.Join(directory, name)
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", executable, "./internal/testcli")
	cmd.Env = os.Environ()
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake Codex CLI: %v\n%s", err, output)
	}
	return executable
}
