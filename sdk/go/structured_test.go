package codex_test

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zhubiaook/codex/sdk/go"
)

type namedPrompt string

type structuredAnswer struct {
	Answer string `json:"answer"`
	Count  int    `json:"count"`
}

func TestThreadRunJSONForwardsStructuredInputAndRemovesSchema(t *testing.T) {
	schema := []byte(`{"type":"object","properties":{"answer":{"type":"string"},"count":{"type":"integer"}},"required":["answer","count"],"additionalProperties":false}`)
	images := []string{filepath.Join("screens", "one.png"), filepath.Join("screens", "two.jpg")}
	statePath := filepath.Join(t.TempDir(), "schema-path")
	expectedImages, err := json.Marshal(images)
	if err != nil {
		t.Fatalf("marshal expected images: %v", err)
	}
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "structured",
			"CODEX_FAKE_STATE":    statePath,
			"EXPECTED_PROMPT":     "Describe the screenshots.",
			"EXPECTED_SCHEMA":     string(schema),
			"EXPECTED_IMAGES":     string(expectedImages),
			"EXPECTED_RESUME_ID":  "persisted-thread",
			"STRUCTURED_RESPONSE": `{"answer":"done","count":2}`,
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	thread, err := client.ResumeThread("persisted-thread", codex.ThreadOptions{})
	if err != nil {
		t.Fatalf("ResumeThread() error = %v", err)
	}

	got, err := thread.RunJSON[structuredAnswer](
		t.Context(),
		codex.StructuredInput{Text: "Describe the screenshots.", LocalImages: images},
		schema,
		codex.TurnOptions{},
	)
	if err != nil {
		t.Fatalf("RunJSON() error = %v", err)
	}
	want := codex.StructuredTurn[structuredAnswer]{
		Turn: codex.Turn{
			Items: []codex.ThreadItem{
				&codex.AgentMessageItem{ID: "item-1", Text: `{"answer":"done","count":2}`},
			},
			FinalResponse: `{"answer":"done","count":2}`,
			Usage:         &codex.Usage{InputTokens: 1, OutputTokens: 1},
		},
		Output: structuredAnswer{Answer: "done", Count: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RunJSON() = %#v, want %#v", got, want)
	}
	schemaPath, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read recorded schema path: %v", err)
	}
	if _, err := os.Stat(string(schemaPath)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("schema file still exists after RunJSON(): %v", err)
	}
}

func TestThreadAcceptsNamedStringAndRawStructuredOutput(t *testing.T) {
	schema := []byte(`{"type":"object"}`)
	client, err := newStructuredClient(t, "structured", schema, `{"answer":"raw"}`)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	turn, err := client.StartThread(codex.ThreadOptions{}).Run(
		t.Context(),
		namedPrompt("structured"),
		codex.TurnOptions{OutputSchema: schema},
	)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if turn.FinalResponse != `{"answer":"raw"}` {
		t.Errorf("Turn.FinalResponse = %q", turn.FinalResponse)
	}
}

func TestThreadRejectsInvalidOutputSchema(t *testing.T) {
	client, err := codex.NewClient(codex.ClientOptions{CodexPath: buildFakeCodex(t)})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	for _, schema := range [][]byte{nil, {}, []byte(`[]`), []byte(`{"type":`)} {
		if schema == nil {
			continue
		}
		_, err := client.StartThread(codex.ThreadOptions{}).Run(
			t.Context(),
			"invalid schema",
			codex.TurnOptions{OutputSchema: schema},
		)
		validationError, ok := errors.AsType[*codex.ValidationError](err)
		if !ok || validationError.Field != "output schema" {
			t.Errorf("Run() error = %T %v, want output schema ValidationError", err, err)
		}
	}
	_, err = client.StartThread(codex.ThreadOptions{}).RunJSON[structuredAnswer](
		t.Context(), "invalid schema", nil, codex.TurnOptions{},
	)
	validationError, ok := errors.AsType[*codex.ValidationError](err)
	if !ok || validationError.Field != "output schema" {
		t.Errorf("RunJSON() nil schema error = %T %v, want output schema ValidationError", err, err)
	}
}

func TestRunJSONReportsTargetTypeMismatch(t *testing.T) {
	schema := []byte(`{"type":"object"}`)
	for _, response := range []string{`{"count":"not-an-integer"}`, `not JSON`} {
		client, err := newStructuredClient(t, "structured", schema, response)
		if err != nil {
			t.Fatalf("NewClient() error = %v", err)
		}
		_, err = client.StartThread(codex.ThreadOptions{}).RunJSON[structuredAnswer](
			t.Context(), "structured", schema, codex.TurnOptions{},
		)
		decodeError, ok := errors.AsType[*codex.OutputDecodeError](err)
		if !ok {
			t.Fatalf("RunJSON() error = %T %v, want *codex.OutputDecodeError", err, err)
		}
		if decodeError.Target != "codex_test.structuredAnswer" {
			t.Errorf("OutputDecodeError.Target = %q", decodeError.Target)
		}
	}
}

func TestOutputSchemaIsRemovedOnFailureCancellationAndEarlyBreak(t *testing.T) {
	schema := []byte(`{"type":"object"}`)
	for _, scenario := range []string{"structured-exit", "structured-cancel", "structured-early-break"} {
		t.Run(scenario, func(t *testing.T) {
			statePath := filepath.Join(t.TempDir(), "schema-path")
			client, err := codex.NewClient(codex.ClientOptions{
				CodexPath: buildFakeCodex(t),
				Env: map[string]string{
					"CODEX_FAKE_SCENARIO": scenario,
					"CODEX_FAKE_STATE":    statePath,
					"EXPECTED_PROMPT":     "structured",
					"EXPECTED_SCHEMA":     string(schema),
					"EXPECTED_IMAGES":     "[]",
				},
			})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			for event, err := range client.StartThread(codex.ThreadOptions{}).RunStreamed(
				ctx, "structured", codex.TurnOptions{OutputSchema: schema},
			) {
				if scenario == "structured-cancel" {
					cancel()
				}
				if scenario == "structured-early-break" && event != nil {
					break
				}
				if err != nil {
					break
				}
			}
			schemaPath, err := os.ReadFile(statePath)
			if err != nil {
				t.Fatalf("read recorded schema path: %v", err)
			}
			if _, err := os.Stat(string(schemaPath)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("schema file still exists: %v", err)
			}
		})
	}
}

func newStructuredClient(t *testing.T, scenario string, schema []byte, response string) (*codex.Client, error) {
	t.Helper()
	return codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": scenario,
			"CODEX_FAKE_STATE":    filepath.Join(t.TempDir(), "schema-path"),
			"EXPECTED_PROMPT":     "structured",
			"EXPECTED_SCHEMA":     string(schema),
			"EXPECTED_IMAGES":     "[]",
			"STRUCTURED_RESPONSE": response,
		},
	})
}
