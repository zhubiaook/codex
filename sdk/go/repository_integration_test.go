package codex_test

import (
	jsonv2 "encoding/json/v2"
	"errors"
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
	executable := repositoryCodexExecutable(t)

	tests := []struct {
		name           string
		authentication codex.ProviderAuthentication
		threadModel    string
		expectedModel  string
		expectedAuth   string
	}{
		{
			name:           "bearer token with provider default model",
			authentication: codex.BearerToken("integration-test-key"),
			expectedModel:  "provider-default-model",
			expectedAuth:   "Bearer integration-test-key",
		},
		{
			name:           "no authentication with thread model override",
			authentication: codex.NoAuthentication(),
			threadModel:    "thread-model",
			expectedModel:  "thread-model",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := make(chan capturedResponsesRequest, 1)
			mux := http.NewServeMux()
			mux.HandleFunc("POST /responses", func(response http.ResponseWriter, request *http.Request) {
				body, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
				if err != nil {
					http.Error(response, err.Error(), http.StatusBadRequest)
					return
				}
				requests <- capturedResponsesRequest{
					body:          body,
					authorization: request.Header.Get("Authorization"),
				}
				response.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(response, repositoryIntegrationSSE)
			})
			server := httptest.NewServer(mux)
			defer server.Close()

			environment := currentEnvironment()
			environment["CODEX_HOME"] = t.TempDir()
			client, err := codex.NewClient(codex.ClientOptions{
				CodexPath: executable,
				Provider: &codex.ResponsesProvider{
					BaseURL:        server.URL,
					Model:          "provider-default-model",
					Authentication: test.authentication,
					Name:           "Go SDK integration test",
				},
				Experimental: &codex.ExperimentalClientOptions{
					ConfigOverrides: []string{"features.plugins=false"},
				},
				Env: environment,
			})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}

			turn, err := startThread(t, client, codex.ThreadOptions{
				Model:            test.threadModel,
				WorkingDirectory: t.TempDir(),
				SandboxMode:      codex.SandboxReadOnly,
				ApprovalPolicy:   codex.ApprovalNever,
			}).Run(t.Context(), "Reply with the integration fixture.", codex.TurnOptions{})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if turn.FinalResponse != "Go SDK integration passed." {
				t.Errorf("Turn.FinalResponse = %q", turn.FinalResponse)
			}

			captured := <-requests
			if captured.authorization != test.expectedAuth {
				t.Errorf("Authorization = %q, want %q", captured.authorization, test.expectedAuth)
			}
			var request responsesRequest
			if err := jsonv2.Unmarshal(captured.body, &request); err != nil {
				t.Fatalf("decode Responses API request: %v", err)
			}
			if request.Model != test.expectedModel {
				t.Errorf("Responses API model = %q, want %q", request.Model, test.expectedModel)
			}
			if !requestContainsText(request, "Reply with the integration fixture.") {
				t.Errorf("Responses API request does not contain the Turn input: %#v", request)
			}
		})
	}
}

func TestRepositoryCodexIntegrationRequiresGitRepository(t *testing.T) {
	executable := repositoryCodexExecutable(t)
	environment := currentEnvironment()
	environment["CODEX_HOME"] = t.TempDir()
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: executable,
		Provider: &codex.ResponsesProvider{
			BaseURL:        "http://127.0.0.1:1",
			Model:          "provider-model",
			Authentication: codex.NoAuthentication(),
		},
		Experimental: &codex.ExperimentalClientOptions{
			ConfigOverrides: []string{"features.plugins=false"},
		},
		Env: environment,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	thread := startThread(t, client, codex.ThreadOptions{
		WorkingDirectory:     t.TempDir(),
		RequireGitRepository: true,
	})
	_, err = thread.Run(t.Context(), "This Turn must not start.", codex.TurnOptions{})
	execError, ok := errors.AsType[*codex.ExecError](err)
	if !ok {
		t.Fatalf("Run() error = %T %v, want *codex.ExecError", err, err)
	}
	if !strings.Contains(execError.Stderr, "Not inside a trusted directory") {
		t.Errorf("ExecError.Stderr = %q, want repository trust error", execError.Stderr)
	}
}

func repositoryCodexExecutable(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the repository Codex integration test runs in Linux CI")
	}
	executable := os.Getenv("CODEX_EXEC_PATH")
	if executable == "" {
		t.Skip("CODEX_EXEC_PATH is not set")
	}
	return executable
}

type capturedResponsesRequest struct {
	body          []byte
	authorization string
}

type responsesRequest struct {
	Model string `json:"model"`
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
