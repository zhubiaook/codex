package codex_test

import (
	json "encoding/json/v2"
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
	rawOverrides := []string{
		"features={}",
		`list=["alpha", 2, true, {"quoted.key" = "value"}]`,
		"nested.count=3",
		"show_raw_agent_reasoning=true",
		`permissions.audit.filesystem={":root"="read"}`,
	}
	environment := map[string]string{
		"CODEX_FAKE_SCENARIO": "args",
		"EXPECTED_ARGS":       string(expectedArgsJSON),
		"EXPECTED_API_KEY":    "sdk-secret",
		"EXPECTED_ORIGINATOR": "codex_sdk_go",
		"EXPECTED_ABSENT":     "HOST_ONLY_FOR_CODEX_TEST",
	}
	t.Setenv("HOST_ONLY_FOR_CODEX_TEST", "must-not-leak")
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		APIKey:    "sdk-secret",
		Experimental: &codex.ExperimentalClientOptions{
			ConfigOverrides: rawOverrides,
		},
		Env: environment,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	rawOverrides[0] = "mutated=true"
	environment["EXPECTED_API_KEY"] = "mutated"

	additionalDirectories := []string{"/shared/one", "/shared/two"}
	thread := startThread(t, client, codex.ThreadOptions{
		Model:                 "gpt-test",
		ThreadSource:          "automated_review",
		SandboxMode:           codex.SandboxWorkspaceWrite,
		WorkingDirectory:      "/workspace",
		AdditionalDirectories: additionalDirectories,
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

func TestProcessErrorDoesNotExposeAPIKey(t *testing.T) {
	const secret = "super-secret-api-key"
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		APIKey:    secret,
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "secret-exit",
			"EXPECTED_PROMPT":     "fail",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = startThread(t, client, codex.ThreadOptions{}).Run(t.Context(), "fail", codex.TurnOptions{})
	if err == nil {
		t.Fatal("Run() error = nil, want process error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("Run() error exposes API key: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Errorf("Run() error does not report redaction: %v", err)
	}
}
