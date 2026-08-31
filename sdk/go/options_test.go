package codex_test

import (
	json "encoding/json/v2"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/zhubiaook/codex/sdk/go"
)

func TestClientAppliesExecutionOptionsInPrecedenceOrder(t *testing.T) {
	expectedArgs := []string{
		"exec",
		"--experimental-json",
		"--config",
		"features={}",
		"--config",
		`list=["alpha", 2, true, {"quoted.key" = "value"}]`,
		"--config",
		"nested.count=3",
		"--config",
		"show_raw_agent_reasoning=true",
		"--config",
		`permissions.audit.filesystem={":root"="read"}`,
		"--config",
		`openai_base_url="https://codex.example.test/v1"`,
		"--model",
		"gpt-test",
		"--thread-source",
		"automated_review",
		"--sandbox",
		"workspace-write",
		"--cd",
		"/workspace",
		"--add-dir",
		"/shared/one",
		"--add-dir",
		"/shared/two",
		"--skip-git-repo-check",
		"--config",
		`model_reasoning_effort="high"`,
		"--config",
		"sandbox_workspace_write.network_access=true",
		"--config",
		`web_search="cached"`,
		"--config",
		`approval_policy="on-request"`,
	}
	expectedArgsJSON, err := json.Marshal(expectedArgs)
	if err != nil {
		t.Fatalf("marshal expected arguments: %v", err)
	}
	config := map[string]any{
		"features": map[string]any{},
		"list": []any{
			"alpha",
			int64(2),
			true,
			map[string]any{"quoted.key": "value"},
		},
		"nested":                   map[string]any{"count": 3},
		"show_raw_agent_reasoning": true,
	}
	rawOverrides := []string{`permissions.audit.filesystem={":root"="read"}`}
	environment := map[string]string{
		"CODEX_FAKE_SCENARIO": "args",
		"EXPECTED_ARGS":       string(expectedArgsJSON),
		"EXPECTED_API_KEY":    "sdk-secret",
		"EXPECTED_ORIGINATOR": "codex_sdk_go",
		"EXPECTED_ABSENT":     "HOST_ONLY_FOR_CODEX_TEST",
	}
	t.Setenv("HOST_ONLY_FOR_CODEX_TEST", "must-not-leak")
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath:       buildFakeCodex(t),
		BaseURL:         "https://codex.example.test/v1",
		APIKey:          "sdk-secret",
		Config:          config,
		ConfigOverrides: rawOverrides,
		Env:             environment,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	config["show_raw_agent_reasoning"] = false
	rawOverrides[0] = "mutated=true"
	environment["EXPECTED_API_KEY"] = "mutated"

	additionalDirectories := []string{"/shared/one", "/shared/two"}
	thread := client.StartThread(codex.ThreadOptions{
		Model:                 "gpt-test",
		ThreadSource:          "automated_review",
		SandboxMode:           codex.SandboxWorkspaceWrite,
		WorkingDirectory:      "/workspace",
		AdditionalDirectories: additionalDirectories,
		SkipGitRepoCheck:      true,
		ModelReasoningEffort:  codex.ReasoningEffortHigh,
		NetworkAccess:         codex.NetworkAccessEnabled,
		WebSearchMode:         codex.WebSearchCached,
		ApprovalPolicy:        codex.ApprovalOnRequest,
	})
	additionalDirectories[0] = "/mutated"

	if _, err := thread.Run(t.Context(), "configured", codex.TurnOptions{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestNewClientReportsConfigValidationPath(t *testing.T) {
	tests := []struct {
		name  string
		value any
		field string
	}{
		{name: "nil", value: nil, field: "nested.value"},
		{name: "non-finite", value: math.Inf(1), field: "nested.value"},
		{name: "unsupported", value: make(chan int), field: "nested.value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := codex.NewClient(codex.ClientOptions{
				CodexPath: buildFakeCodex(t),
				Config: map[string]any{
					"nested": map[string]any{"value": test.value},
				},
			})
			validationError, ok := errors.AsType[*codex.ValidationError](err)
			if !ok {
				t.Fatalf("NewClient() error = %T %v, want *codex.ValidationError", err, err)
			}
			if validationError.Field != test.field {
				t.Errorf("ValidationError.Field = %q, want %q", validationError.Field, test.field)
			}
		})
	}
}

func TestProcessErrorDoesNotExposeAPIKey(t *testing.T) {
	const secret = "super-secret-api-key"
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		APIKey:    secret,
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "exit",
			"EXPECTED_PROMPT":     "fail",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.StartThread(codex.ThreadOptions{}).Run(t.Context(), "fail", codex.TurnOptions{})
	if err == nil {
		t.Fatal("Run() error = nil, want process error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("Run() error exposes API key: %v", err)
	}
}
