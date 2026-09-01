package codex_test

import (
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/zhubiaook/codex/sdk/go"
)

func TestRepositoryCodexIntegration(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the repository Codex integration test runs in Linux CI")
	}
	executable := os.Getenv("CODEX_EXEC_PATH")
	if executable == "" {
		t.Skip("CODEX_EXEC_PATH is not set")
	}

	requests := make(chan []byte, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /responses", func(response http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- body
		response.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(response, repositoryIntegrationSSE)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	environment := currentEnvironment()
	environment["CODEX_HOME"] = t.TempDir()
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: executable,
		APIKey:    "integration-test-key",
		Env:       environment,
		Config: map[string]any{
			"model_provider": "go_sdk_mock",
			"model_providers": map[string]any{
				"go_sdk_mock": map[string]any{
					"name":                "Go SDK integration test",
					"base_url":            server.URL,
					"env_key":             "CODEX_API_KEY",
					"wire_api":            "responses",
					"supports_websockets": false,
				},
			},
			"features": map[string]any{"plugins": false},
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	turn, err := client.StartThread(codex.ThreadOptions{
		Model:            "gpt-5.1",
		WorkingDirectory: t.TempDir(),
		SkipGitRepoCheck: true,
		SandboxMode:      codex.SandboxReadOnly,
		ApprovalPolicy:   codex.ApprovalNever,
	}).Run(t.Context(), "Reply with the integration fixture.", codex.TurnOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if turn.FinalResponse != "Go SDK integration passed." {
		t.Errorf("Turn.FinalResponse = %q", turn.FinalResponse)
	}

	var request responsesRequest
	if err := json.Unmarshal(<-requests, &request); err != nil {
		t.Fatalf("decode Responses API request: %v", err)
	}
	if !requestContainsText(request, "Reply with the integration fixture.") {
		t.Errorf("Responses API request does not contain the Turn input: %#v", request)
	}
}

type responsesRequest struct {
	Input []struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"input"`
}

func requestContainsText(request responsesRequest, text string) bool {
	for _, message := range request.Input {
		if message.Role != "user" {
			continue
		}
		for _, content := range message.Content {
			if content.Type == "input_text" && content.Text == text {
				return true
			}
		}
	}
	return false
}

func currentEnvironment() map[string]string {
	environment := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			environment[key] = value
		}
	}
	return environment
}

const repositoryIntegrationSSE = `event: response.created
data: {"type":"response.created","response":{"id":"response-go-sdk"}}

event: response.output_item.done
data: {"type":"response.output_item.done","item":{"type":"message","role":"assistant","id":"message-go-sdk","content":[{"type":"output_text","text":"Go SDK integration passed."}]}}

event: response.completed
data: {"type":"response.completed","response":{"id":"response-go-sdk","usage":{"input_tokens":5,"input_tokens_details":{"cached_tokens":0},"output_tokens":5,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":10}}}

`
