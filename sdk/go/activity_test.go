package codex_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/zhubiaook/codex/sdk/go"
)

func TestThreadDecodesAgentActivityAndUnknownVariants(t *testing.T) {
	client := newActivityClient(t, "activity")
	var got []codex.ThreadEvent
	for event, err := range client.StartThread(codex.ThreadOptions{}).RunStreamed(
		t.Context(), "activity", codex.TurnOptions{},
	) {
		if err != nil {
			t.Fatalf("RunStreamed() error = %v", err)
		}
		got = append(got, event)
	}
	zero := 0
	want := []codex.ThreadEvent{
		&codex.ThreadStartedEvent{ThreadID: "thread-activity"},
		&codex.TurnStartedEvent{},
		&codex.ItemStartedEvent{Item: &codex.CommandExecutionItem{
			ID: "command-1", Command: "go test ./...", AggregatedOutput: "", Status: codex.CommandInProgress,
		}},
		&codex.ItemUpdatedEvent{Item: &codex.CommandExecutionItem{
			ID: "command-1", Command: "go test ./...", AggregatedOutput: "ok\n", Status: codex.CommandInProgress,
		}},
		&codex.ItemCompletedEvent{Item: &codex.CommandExecutionItem{
			ID: "command-1", Command: "go test ./...", AggregatedOutput: "ok\n", ExitCode: &zero, Status: codex.CommandCompleted,
		}},
		&codex.ItemCompletedEvent{Item: &codex.ReasoningItem{ID: "reasoning-1", Text: "Inspect the failure."}},
		&codex.ItemCompletedEvent{Item: &codex.FileChangeItem{
			ID: "patch-1",
			Changes: []codex.FileUpdateChange{
				{Path: "new.go", Kind: codex.PatchChangeAdd},
				{Path: "old.go", Kind: codex.PatchChangeDelete},
				{Path: "changed.go", Kind: codex.PatchChangeUpdate},
			},
			Status: codex.PatchApplyCompleted,
		}},
		&codex.ItemCompletedEvent{Item: &codex.AgentMessageItem{ID: "message-1", Text: "Done."}},
		&codex.UnknownEvent{
			UnknownType: "future.event",
			Raw:         json.RawMessage(`{"type":"future.event","answer":42}`),
		},
		&codex.ItemCompletedEvent{Item: &codex.UnknownItem{
			ID:          "future-1",
			UnknownType: "future_item",
			Raw:         json.RawMessage(`{"id":"future-1","type":"future_item","payload":{"ok":true}}`),
		}},
		&codex.TurnCompletedEvent{Usage: codex.Usage{InputTokens: 4, CachedInputTokens: 2, OutputTokens: 3, ReasoningOutputTokens: 1}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Thread Events = %#v, want %#v", got, want)
	}

	buffered := newActivityClient(t, "activity").StartThread(codex.ThreadOptions{})
	turn, err := buffered.Run(t.Context(), "activity", codex.TurnOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	wantItems := []codex.ThreadItem{
		want[4].(*codex.ItemCompletedEvent).Item,
		want[5].(*codex.ItemCompletedEvent).Item,
		want[6].(*codex.ItemCompletedEvent).Item,
		want[7].(*codex.ItemCompletedEvent).Item,
		want[9].(*codex.ItemCompletedEvent).Item,
	}
	if !reflect.DeepEqual(turn.Items, wantItems) {
		t.Errorf("Turn.Items = %#v, want %#v", turn.Items, wantItems)
	}
	if turn.FinalResponse != "Done." {
		t.Errorf("Turn.FinalResponse = %q", turn.FinalResponse)
	}
}

func TestThreadDecodesDeclinedCommand(t *testing.T) {
	turn, err := newActivityClient(t, "command-declined").StartThread(codex.ThreadOptions{}).Run(
		t.Context(), "activity", codex.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := []codex.ThreadItem{&codex.CommandExecutionItem{
		ID: "command-declined", Command: "git status", AggregatedOutput: "", Status: codex.CommandDeclined,
	}}
	if !reflect.DeepEqual(turn.Items, want) {
		t.Errorf("Turn.Items = %#v, want %#v", turn.Items, want)
	}
}

func TestThreadDecodesFileChangeInProgress(t *testing.T) {
	turn, err := newActivityClient(t, "file-change-in-progress").StartThread(codex.ThreadOptions{}).Run(
		t.Context(), "activity", codex.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := []codex.ThreadItem{&codex.FileChangeItem{
		ID: "file-change-in-progress",
		Changes: []codex.FileUpdateChange{
			{Path: "main.go", Kind: codex.PatchChangeUpdate},
		},
		Status: codex.PatchApplyInProgress,
	}}
	if !reflect.DeepEqual(turn.Items, want) {
		t.Errorf("Turn.Items = %#v, want %#v", turn.Items, want)
	}
}

func TestKnownMalformedActivityIsNotDowngradedToUnknown(t *testing.T) {
	client := newActivityClient(t, "malformed-known")
	_, err := client.StartThread(codex.ThreadOptions{}).Run(t.Context(), "activity", codex.TurnOptions{})
	if _, ok := errors.AsType[*codex.DecodeError](err); !ok {
		t.Fatalf("Run() error = %T %v, want *codex.DecodeError", err, err)
	}
}

func TestTurnAndThreadFailuresAreTyped(t *testing.T) {
	turnClient := newActivityClient(t, "turn-failed")
	_, err := turnClient.StartThread(codex.ThreadOptions{}).Run(t.Context(), "activity", codex.TurnOptions{})
	turnError, ok := errors.AsType[*codex.TurnFailedError](err)
	if !ok || turnError.ThreadError.Message != "model failed" {
		t.Fatalf("Run() error = %T %v, want model TurnFailedError", err, err)
	}

	streamClient := newActivityClient(t, "thread-error")
	_, err = streamClient.StartThread(codex.ThreadOptions{}).Run(t.Context(), "activity", codex.TurnOptions{})
	streamError, ok := errors.AsType[*codex.ThreadErrorEvent](err)
	if !ok || streamError.Message != "stream failed" {
		t.Fatalf("Run() error = %T %v, want ThreadErrorEvent", err, err)
	}
}

func newActivityClient(t *testing.T, scenario string) *codex.Client {
	t.Helper()
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": scenario,
			"EXPECTED_PROMPT":     "activity",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}
