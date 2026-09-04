package codex

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"
)

const (
	internalOriginatorEnv = "CODEX_INTERNAL_ORIGINATOR_OVERRIDE"
	goSDKOriginator       = "codex_sdk_go"
	apiKeyEnv             = "CODEX_API_KEY"
)

// ClientOptions configures a Client.
type ClientOptions struct {
	// CodexPath is the path to the Codex CLI executable. When empty, NewClient
	// resolves codex from PATH.
	CodexPath string
	// APIKey is provided to the built-in OpenAI provider through CODEX_API_KEY.
	APIKey string
	// Provider configures a Responses-compatible Provider. When nil, the built-in
	// OpenAI provider is used.
	Provider *ResponsesProvider
	// Experimental contains unmodeled Codex CLI configuration without
	// compatibility guarantees.
	Experimental *ExperimentalClientOptions
	// Env is the exact environment passed to the Codex CLI. A nil map snapshots
	// the current process environment when the Client is created.
	Env map[string]string
}

// Client starts and resumes Codex Threads. A Client is safe for concurrent use.
type Client struct {
	executable      string
	defaultModel    string
	configOverrides []string
	environment     []string
	apiKey          string
}

// NewClient validates its configuration, creates a Client, and resolves its
// Codex CLI executable.
func NewClient(options ClientOptions) (*Client, error) {
	if options.APIKey != "" {
		if err := validateConfigString("apiKey", options.APIKey); err != nil {
			return nil, err
		}
	}
	apiKey := options.APIKey
	defaultModel := ""
	var configOverrides []string
	if options.Experimental != nil {
		configOverrides = slices.Clone(options.Experimental.ConfigOverrides)
	}
	if err := validateExperimentalOverrides(configOverrides); err != nil {
		return nil, err
	}
	if options.Provider != nil {
		if options.APIKey != "" {
			return nil, newValidationError(
				"apiKey",
				"must not be set with provider authentication",
			)
		}
		provider, err := normalizeResponsesProvider(*options.Provider)
		if err != nil {
			return nil, err
		}
		apiKey = provider.apiKey
		defaultModel = provider.model
		configOverrides = append(provider.overrides, configOverrides...)
	}

	executable := options.CodexPath
	if executable == "" {
		executable = "codex"
	}
	resolved, err := exec.LookPath(executable)
	if err != nil {
		return nil, &ExecutableError{Path: executable, Err: err}
	}
	return &Client{
		executable:      resolved,
		defaultModel:    defaultModel,
		configOverrides: configOverrides,
		environment:     snapshotEnvironment(options.Env, apiKey),
		apiKey:          strings.Clone(apiKey),
	}, nil
}

// StartThread validates its options and creates a new Thread.
func (c *Client) StartThread(options ThreadOptions) (*Thread, error) {
	options, err := c.normalizeThreadOptions(options)
	if err != nil {
		return nil, err
	}
	return &Thread{client: c, options: options}, nil
}

// ResumeThread reconstructs a persisted Thread from its identifier.
func (c *Client) ResumeThread(id string, options ThreadOptions) (*Thread, error) {
	if id == "" {
		return nil, &ValidationError{Field: "id", Err: errors.New("must not be empty")}
	}
	options, err := c.normalizeThreadOptions(options)
	if err != nil {
		return nil, err
	}
	return &Thread{client: c, options: options, id: id}, nil
}

func (c *Client) normalizeThreadOptions(options ThreadOptions) (ThreadOptions, error) {
	if options.Model != "" {
		if err := validateConfigString("model", options.Model); err != nil {
			return ThreadOptions{}, err
		}
	} else {
		options.Model = c.defaultModel
	}
	if options.ThreadSource != "" {
		if err := validateConfigString("threadSource", options.ThreadSource); err != nil {
			return ThreadOptions{}, err
		}
	}
	if strings.ContainsRune(options.WorkingDirectory, 0) {
		return ThreadOptions{}, newValidationError("workingDirectory", "must not contain NUL")
	}
	for index, directory := range options.AdditionalDirectories {
		if strings.ContainsRune(directory, 0) {
			return ThreadOptions{}, newValidationError(
				fmt.Sprintf("additionalDirectories[%d]", index),
				"must not contain NUL",
			)
		}
	}
	switch options.SandboxMode {
	case "", SandboxReadOnly, SandboxWorkspaceWrite, SandboxDangerFullAccess:
	default:
		return ThreadOptions{}, invalidThreadOption("sandboxMode", string(options.SandboxMode))
	}
	switch options.ModelReasoningEffort {
	case "", ReasoningEffortMinimal, ReasoningEffortLow, ReasoningEffortMedium,
		ReasoningEffortHigh, ReasoningEffortXHigh, ReasoningEffortMax,
		ReasoningEffortUltra, ReasoningEffortPersistent:
	default:
		return ThreadOptions{}, invalidThreadOption("modelReasoningEffort", string(options.ModelReasoningEffort))
	}
	switch options.NetworkAccess {
	case "", NetworkAccessEnabled, NetworkAccessDisabled:
	default:
		return ThreadOptions{}, invalidThreadOption("networkAccess", string(options.NetworkAccess))
	}
	switch options.WebSearchMode {
	case "", WebSearchDisabled, WebSearchCached, WebSearchLive:
	default:
		return ThreadOptions{}, invalidThreadOption("webSearchMode", string(options.WebSearchMode))
	}
	switch options.ApprovalPolicy {
	case "", ApprovalNever, ApprovalOnRequest, ApprovalOnFailure, ApprovalUntrusted:
	default:
		return ThreadOptions{}, invalidThreadOption("approvalPolicy", string(options.ApprovalPolicy))
	}
	return snapshotThreadOptions(options), nil
}

func invalidThreadOption(field string, value string) error {
	return newValidationError(field, fmt.Sprintf("unsupported value %q", value))
}

func snapshotEnvironment(environment map[string]string, apiKey string) []string {
	var cloned map[string]string
	if environment == nil {
		cloned = make(map[string]string)
		for _, entry := range os.Environ() {
			key, value, ok := splitEnvironmentEntry(entry)
			if ok {
				cloned[key] = value
			}
		}
	} else {
		cloned = maps.Clone(environment)
	}
	if _, ok := cloned[internalOriginatorEnv]; !ok {
		cloned[internalOriginatorEnv] = goSDKOriginator
	}
	if apiKey != "" {
		cloned[apiKeyEnv] = apiKey
	}
	keys := slices.Sorted(maps.Keys(cloned))
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+cloned[key])
	}
	return result
}

func splitEnvironmentEntry(entry string) (string, string, bool) {
	if remainder, ok := strings.CutPrefix(entry, "="); ok {
		key, value, ok := strings.Cut(remainder, "=")
		return "=" + key, value, ok
	}
	return strings.Cut(entry, "=")
}
