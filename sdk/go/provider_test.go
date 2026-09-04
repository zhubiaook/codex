package codex_test

import (
	json "encoding/json/v2"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhubiaook/codex/sdk/go"
)

func TestResponsesProviderRunsWithDefaults(t *testing.T) {
	expectedArgs := []string{
		"exec",
		"--experimental-json",
		"--config",
		`model_provider="go_sdk_provider"`,
		"--config",
		`model_providers.go_sdk_provider.base_url="http://provider.example.test/v1"`,
		"--config",
		`model_providers.go_sdk_provider.env_key="CODEX_API_KEY"`,
		"--config",
		`model_providers.go_sdk_provider.name="provider.example.test"`,
		"--model",
		"provider-model",
		"--skip-git-repo-check",
	}
	expectedArgsJSON, err := json.Marshal(expectedArgs)
	if err != nil {
		t.Fatalf("marshal expected arguments: %v", err)
	}
	provider := &codex.ResponsesProvider{
		BaseURL:        "http://provider.example.test/v1/",
		Model:          "provider-model",
		Authentication: codex.BearerToken("provider-secret"),
	}
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Provider:  provider,
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "args",
			"EXPECTED_ARGS":       string(expectedArgsJSON),
			"EXPECTED_API_KEY":    "provider-secret",
			"EXPECTED_ORIGINATOR": "codex_sdk_go",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	provider.BaseURL = "https://mutated.example.test"
	provider.Model = "mutated-model"
	provider.Authentication = codex.NoAuthentication()
	thread, err := client.StartThread(codex.ThreadOptions{})
	if err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}

	turn, err := thread.Run(t.Context(), "configured", codex.TurnOptions{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if turn.FinalResponse != "configured" {
		t.Errorf("Turn.FinalResponse = %q, want %q", turn.FinalResponse, "configured")
	}
}

func TestResponsesProviderAppliesOptionalCapabilitiesAndThreadModel(t *testing.T) {
	expectedArgs := []string{
		"exec",
		"--experimental-json",
		"--config",
		`model_provider="go_sdk_provider"`,
		"--config",
		`model_providers.go_sdk_provider.base_url="https://provider.example.test/v1"`,
		"--config",
		`model_providers.go_sdk_provider.name="Example Provider"`,
		"--config",
		"model_providers.go_sdk_provider.supports_websockets=true",
		"--config",
		"features.plugins=false",
		"--model",
		"thread-model",
	}
	expectedArgsJSON, err := json.Marshal(expectedArgs)
	if err != nil {
		t.Fatalf("marshal expected arguments: %v", err)
	}
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Provider: &codex.ResponsesProvider{
			BaseURL:            "https://provider.example.test/v1",
			Model:              "provider-model",
			Authentication:     codex.NoAuthentication(),
			Name:               "Example Provider",
			SupportsWebSockets: true,
		},
		Experimental: &codex.ExperimentalClientOptions{
			ConfigOverrides: []string{"features.plugins=false"},
		},
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "args",
			"EXPECTED_ARGS":       string(expectedArgsJSON),
			"EXPECTED_ORIGINATOR": "codex_sdk_go",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	thread, err := client.StartThread(codex.ThreadOptions{
		Model:                "thread-model",
		RequireGitRepository: true,
	})
	if err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}

	if _, err := thread.Run(t.Context(), "configured", codex.TurnOptions{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestNewClientValidatesResponsesProviderBeforeResolvingExecutable(t *testing.T) {
	validProvider := func() *codex.ResponsesProvider {
		return &codex.ResponsesProvider{
			BaseURL:        "https://provider.example.test/v1",
			Model:          "provider-model",
			Authentication: codex.BearerToken("provider-secret"),
		}
	}
	tests := []struct {
		name    string
		options func() codex.ClientOptions
		field   string
	}{
		{
			name: "empty base URL",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.BaseURL = ""
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.baseURL",
		},
		{
			name: "base URL surrounding whitespace",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.BaseURL = " https://provider.example.test/v1"
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.baseURL",
		},
		{
			name: "relative base URL",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.BaseURL = "/v1"
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.baseURL",
		},
		{
			name: "base URL without host name",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.BaseURL = "http://:8080/v1"
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.baseURL",
		},
		{
			name: "unsupported base URL scheme",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.BaseURL = "ftp://provider.example.test/v1"
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.baseURL",
		},
		{
			name: "base URL user information",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.BaseURL = "https://user:pass@provider.example.test/v1"
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.baseURL",
		},
		{
			name: "base URL query",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.BaseURL = "https://provider.example.test/v1?region=test"
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.baseURL",
		},
		{
			name: "base URL fragment",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.BaseURL = "https://provider.example.test/v1#fragment"
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.baseURL",
		},
		{
			name: "base URL final responses route",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.BaseURL = "https://provider.example.test/v1/responses"
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.baseURL",
		},
		{
			name: "base URL final responses route and slash",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.BaseURL = "https://provider.example.test/v1/responses/"
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.baseURL",
		},
		{
			name: "empty model",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.Model = ""
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.model",
		},
		{
			name: "model surrounding whitespace",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.Model = " provider-model"
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.model",
		},
		{
			name: "name surrounding whitespace",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.Name = " Provider"
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.name",
		},
		{
			name: "unset authentication",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.Authentication = codex.ProviderAuthentication{}
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.authentication",
		},
		{
			name: "empty bearer token",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.Authentication = codex.BearerToken("")
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.authentication.bearerToken",
		},
		{
			name: "bearer token surrounding whitespace",
			options: func() codex.ClientOptions {
				provider := validProvider()
				provider.Authentication = codex.BearerToken(" provider-secret")
				return codex.ClientOptions{Provider: provider}
			},
			field: "provider.authentication.bearerToken",
		},
		{
			name: "built-in API key surrounding whitespace",
			options: func() codex.ClientOptions {
				return codex.ClientOptions{APIKey: " openai-secret"}
			},
			field: "apiKey",
		},
		{
			name: "built-in API key conflict",
			options: func() codex.ClientOptions {
				return codex.ClientOptions{APIKey: "openai-secret", Provider: validProvider()}
			},
			field: "apiKey",
		},
		{
			name: "experimental provider override",
			options: func() codex.ClientOptions {
				return codex.ClientOptions{
					Provider: validProvider(),
					Experimental: &codex.ExperimentalClientOptions{
						ConfigOverrides: []string{`model_providers.other.base_url="https://other.test"`},
					},
				}
			},
			field: "experimental.configOverrides[0]",
		},
		{
			name: "quoted experimental provider override",
			options: func() codex.ClientOptions {
				return codex.ClientOptions{
					Provider: validProvider(),
					Experimental: &codex.ExperimentalClientOptions{
						ConfigOverrides: []string{`"model_provider"="other"`},
					},
				}
			},
			field: "experimental.configOverrides[0]",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := test.options()
			options.CodexPath = filepath.Join(t.TempDir(), "missing-codex")
			_, err := codex.NewClient(options)
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

func TestThreadCreationValidatesOptions(t *testing.T) {
	client, err := codex.NewClient(codex.ClientOptions{CodexPath: buildFakeCodex(t)})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	tests := []struct {
		name    string
		options codex.ThreadOptions
		field   string
	}{
		{name: "model", options: codex.ThreadOptions{Model: " model"}, field: "model"},
		{name: "thread source", options: codex.ThreadOptions{ThreadSource: " source"}, field: "threadSource"},
		{name: "working directory", options: codex.ThreadOptions{WorkingDirectory: "bad\x00path"}, field: "workingDirectory"},
		{name: "additional directory", options: codex.ThreadOptions{AdditionalDirectories: []string{"bad\x00path"}}, field: "additionalDirectories[0]"},
		{name: "sandbox mode", options: codex.ThreadOptions{SandboxMode: codex.SandboxMode("invalid")}, field: "sandboxMode"},
		{name: "reasoning effort", options: codex.ThreadOptions{ModelReasoningEffort: codex.ModelReasoningEffort("invalid")}, field: "modelReasoningEffort"},
		{name: "network access", options: codex.ThreadOptions{NetworkAccess: codex.NetworkAccessMode("invalid")}, field: "networkAccess"},
		{name: "web search", options: codex.ThreadOptions{WebSearchMode: codex.WebSearchMode("invalid")}, field: "webSearchMode"},
		{name: "approval policy", options: codex.ThreadOptions{ApprovalPolicy: codex.ApprovalPolicy("invalid")}, field: "approvalPolicy"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.StartThread(test.options)
			validationError, ok := errors.AsType[*codex.ValidationError](err)
			if !ok {
				t.Fatalf("StartThread() error = %T %v, want *codex.ValidationError", err, err)
			}
			if validationError.Field != test.field {
				t.Errorf("ValidationError.Field = %q, want %q", validationError.Field, test.field)
			}
		})
	}

	_, err = client.ResumeThread("thread-id", codex.ThreadOptions{NetworkAccess: codex.NetworkAccessMode("invalid")})
	validationError, ok := errors.AsType[*codex.ValidationError](err)
	if !ok {
		t.Fatalf("ResumeThread() error = %T %v, want *codex.ValidationError", err, err)
	}
	if validationError.Field != "networkAccess" {
		t.Errorf("ValidationError.Field = %q, want %q", validationError.Field, "networkAccess")
	}
}

func TestResponsesProviderProcessErrorRedactsBearerToken(t *testing.T) {
	const secret = "provider-super-secret"
	client, err := codex.NewClient(codex.ClientOptions{
		CodexPath: buildFakeCodex(t),
		Provider: &codex.ResponsesProvider{
			BaseURL:        "https://provider.example.test/v1",
			Model:          "provider-model",
			Authentication: codex.BearerToken(secret),
		},
		Env: map[string]string{
			"CODEX_FAKE_SCENARIO": "secret-exit",
			"EXPECTED_PROMPT":     "fail",
		},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	thread, err := client.StartThread(codex.ThreadOptions{})
	if err != nil {
		t.Fatalf("StartThread() error = %v", err)
	}
	_, err = thread.Run(t.Context(), "fail", codex.TurnOptions{})
	if err == nil {
		t.Fatal("Run() error = nil, want process error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("Run() error exposes bearer token: %v", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Errorf("Run() error does not report redaction: %v", err)
	}
}
