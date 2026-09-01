package codex

import (
	"errors"
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
	// BaseURL overrides the OpenAI API base URL used by the Codex CLI.
	BaseURL string
	// APIKey is provided to the Codex CLI through CODEX_API_KEY.
	APIKey string
	// Config contains structured Codex configuration. NewClient recursively
	// validates and snapshots TOML-compatible values.
	Config map[string]any
	// ConfigOverrides contains ordered raw key=value overrides passed after
	// structured Config values.
	ConfigOverrides []string
	// Env is the exact environment passed to the Codex CLI. A nil map snapshots
	// the current process environment when the Client is created.
	Env map[string]string
}

// Client starts and resumes Codex Threads. A Client is safe for concurrent use.
type Client struct {
	executable      string
	baseURL         string
	configOverrides []string
	environment     []string
	apiKey          string
}

// NewClient creates a Client and resolves its Codex CLI executable.
func NewClient(options ClientOptions) (*Client, error) {
	executable := options.CodexPath
	if executable == "" {
		executable = "codex"
	}
	resolved, err := exec.LookPath(executable)
	if err != nil {
		return nil, &ExecutableError{Path: executable, Err: err}
	}
	structuredOverrides, err := serializeConfig(options.Config)
	if err != nil {
		return nil, err
	}
	configOverrides := append(structuredOverrides, slices.Clone(options.ConfigOverrides)...)

	return &Client{
		executable:      resolved,
		baseURL:         options.BaseURL,
		configOverrides: configOverrides,
		environment:     snapshotEnvironment(options.Env, options.APIKey),
		apiKey:          strings.Clone(options.APIKey),
	}, nil
}

// StartThread creates a new Thread.
func (c *Client) StartThread(options ThreadOptions) *Thread {
	return &Thread{client: c, options: snapshotThreadOptions(options)}
}

// ResumeThread reconstructs a persisted Thread from its identifier.
func (c *Client) ResumeThread(id string, options ThreadOptions) (*Thread, error) {
	if id == "" {
		return nil, &ValidationError{Field: "id", Err: errors.New("must not be empty")}
	}
	return &Thread{client: c, options: snapshotThreadOptions(options), id: id}, nil
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
