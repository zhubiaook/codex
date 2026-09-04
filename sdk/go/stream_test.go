package codex_test

import (
	"context"
	"errors"
	"iter"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/zhubiaook/codex/sdk/go"
)

func TestRunStreamedIsLazyAndYieldsThreadEvents(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "stream-started")
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "stream",
			"CODEX_FAKE_STATE":    statePath,
			"EXPECTED_PROMPT":     "stream this",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	thread := startThread(t, client, codex.ThreadOptions{})

	stream := thread.RunStreamed(t.Context(), "stream this", codex.TurnOptions{})
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("RunStreamed() started the process before iteration")
	}

	var got []codex.ThreadEvent
	for event, err := range stream {
		if err != nil {
			t.Fatalf("stream error = %v", err)
		}
		got = append(got, event)
	}
	want := []codex.ThreadEvent{
		&codex.ThreadStartedEvent{ThreadID: "thread-stream"},
		&codex.TurnStartedEvent{},
		&codex.ItemCompletedEvent{
			Item: &codex.AgentMessageItem{ID: "item-1", Text: "stream response"},
		},
		&codex.TurnCompletedEvent{Usage: codex.Usage{
			InputTokens:           1,
			CachedInputTokens:     0,
			CacheWriteInputTokens: 0,
			OutputTokens:          1,
			ReasoningOutputTokens: 0,
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Thread Events = %#v, want %#v", got, want)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Errorf("fake Codex process did not start: %v", err)
	}
}

func TestRunStreamedAppliesConsumerBackpressure(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "backpressure-state")
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "backpressure",
			"CODEX_FAKE_STATE":    statePath,
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	stream := startThread(t, client, codex.ThreadOptions{}).RunStreamed(
		t.Context(),
		"backpressure",
		codex.TurnOptions{},
	)
	next, stop := iter.Pull2(stream)
	defer stop()

	event, err, ok := next()
	if !ok || err != nil {
		t.Fatalf("first stream value = (%T, %v, %v)", event, err, ok)
	}
	if _, ok := event.(*codex.ThreadStartedEvent); !ok {
		t.Fatalf("first Thread Event = %T, want *codex.ThreadStartedEvent", event)
	}
	waitForFileContents(t, statePath, "writing")
	time.Sleep(100 * time.Millisecond)
	contents, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read backpressure state: %v", err)
	}
	if string(contents) != "writing" {
		t.Fatalf("producer advanced while consumer was paused: state = %q", contents)
	}

	event, err, ok = next()
	if !ok || err != nil {
		t.Fatalf("second stream value = (%T, %v, %v)", event, err, ok)
	}
	if _, ok := event.(*codex.UnknownEvent); !ok {
		t.Fatalf("second Thread Event = %T, want *codex.UnknownEvent", event)
	}
	waitForFileContents(t, statePath, "written")
}

func waitForFileContents(t *testing.T, path string, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		contents, err := os.ReadFile(path)
		if err == nil && string(contents) == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	contents, err := os.ReadFile(path)
	t.Fatalf("file %q = %q, %v; want %q", path, contents, err, want)
}

func TestRunStreamedEarlyBreakCleansUpAndReleasesThread(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "early-break-state")
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "early-break",
			"CODEX_FAKE_STATE":    statePath,
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	thread := startThread(t, client, codex.ThreadOptions{})

	for event, err := range thread.RunStreamed(
		t.Context(),
		"first",
		codex.TurnOptions{},
	) {
		if err != nil {
			t.Fatalf("first stream error = %v", err)
		}
		if _, ok := event.(*codex.ThreadStartedEvent); !ok {
			t.Fatalf("first Thread Event = %T, want *codex.ThreadStartedEvent", event)
		}
		break
	}

	turn, err := thread.Run(t.Context(), "second", codex.TurnOptions{})
	if err != nil {
		t.Fatalf("Run() after early break error = %v", err)
	}
	if turn.FinalResponse != "after early break" {
		t.Errorf("Turn.FinalResponse = %q", turn.FinalResponse)
	}
}

func TestRunStreamedPreservesContextCancellation(t *testing.T) {
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "cancel",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	var streamErr error
	for event, err := range startThread(t, client, codex.ThreadOptions{}).RunStreamed(
		ctx,
		"cancel",
		codex.TurnOptions{},
	) {
		if err != nil {
			streamErr = err
			break
		}
		if _, ok := event.(*codex.ThreadStartedEvent); ok {
			cancel()
		}
	}
	if !errors.Is(streamErr, context.Canceled) {
		t.Errorf("stream error = %v, want context.Canceled", streamErr)
	}
}

func TestRunStreamedCanOnlyBeConsumedOnce(t *testing.T) {
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "stream",
			"CODEX_FAKE_STATE":    filepath.Join(t.TempDir(), "stream-started"),
			"EXPECTED_PROMPT":     "once",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	stream := startThread(t, client, codex.ThreadOptions{}).RunStreamed(
		t.Context(),
		"once",
		codex.TurnOptions{},
	)
	for _, err := range stream {
		if err != nil {
			t.Fatalf("first iteration error = %v", err)
		}
	}

	var secondErr error
	for _, err := range stream {
		secondErr = err
	}
	if !errors.Is(secondErr, codex.ErrStreamConsumed) {
		t.Errorf("second iteration error = %v, want ErrStreamConsumed", secondErr)
	}
}

func TestThreadRejectsConcurrentTurn(t *testing.T) {
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "cancel",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	thread := startThread(t, client, codex.ThreadOptions{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	firstDone := make(chan error, 1)
	var turns sync.WaitGroup
	turns.Go(func() {
		for event, err := range thread.RunStreamed(ctx, "first", codex.TurnOptions{}) {
			if err != nil {
				firstDone <- err
				return
			}
			if _, ok := event.(*codex.ThreadStartedEvent); ok {
				close(started)
			}
		}
		firstDone <- nil
	})
	<-started

	_, err = thread.Run(t.Context(), "second", codex.TurnOptions{})
	if !errors.Is(err, codex.ErrTurnInProgress) {
		t.Errorf("concurrent Run() error = %v, want ErrTurnInProgress", err)
	}
	cancel()
	turns.Wait()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Errorf("first stream error = %v, want context.Canceled", err)
	}
}

func TestSeparateThreadsRunConcurrently(t *testing.T) {
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "success",
			"EXPECTED_PROMPT":     "parallel",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	results := make(chan error, 2)
	var turns sync.WaitGroup
	for range 2 {
		thread := startThread(t, client, codex.ThreadOptions{})
		turns.Go(func() {
			_, err := thread.Run(t.Context(), "parallel", codex.TurnOptions{})
			results <- err
		})
	}
	turns.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Errorf("Run() error = %v", err)
		}
	}
}

func TestRunStreamedPreservesContextDeadline(t *testing.T) {
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "cancel",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	var streamErr error
	for _, err := range startThread(t, client, codex.ThreadOptions{}).RunStreamed(
		ctx,
		"deadline",
		codex.TurnOptions{},
	) {
		if err != nil {
			streamErr = err
		}
	}
	if !errors.Is(streamErr, context.DeadlineExceeded) {
		t.Errorf("stream error = %v, want context.DeadlineExceeded", streamErr)
	}
}

func TestThreadIsReleasedAfterTerminalErrors(t *testing.T) {
	for _, scenario := range []string{"exit", "malformed"} {
		t.Run(scenario, func(t *testing.T) {
			client, err := codex.NewClient(codex.ClientOptions{
				CodexPath: buildFakeCodex(t),
				Env: map[string]string{
					"CODEX_FAKE_SCENARIO": scenario,
				},
			})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			thread := startThread(t, client, codex.ThreadOptions{})
			if _, err := thread.Run(t.Context(), "first", codex.TurnOptions{}); err == nil {
				t.Fatal("first Run() error = nil")
			}
			if _, err := thread.Run(t.Context(), "second", codex.TurnOptions{}); errors.Is(err, codex.ErrTurnInProgress) {
				t.Errorf("second Run() error = %v, Thread was not released", err)
			}
		})
	}
}
