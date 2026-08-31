package codex

import (
	"errors"
	"maps"
	"os"
	"os/exec"
	"slices"
)

// ClientOptions configures a Client.
type ClientOptions struct {
	// CodexPath is the path to the Codex CLI executable. When empty, NewClient
	// resolves codex from PATH.
	CodexPath string
	// Env is the exact environment passed to the Codex CLI. A nil map snapshots
	// the current process environment when the Client is created.
	Env map[string]string
}

// Client starts and resumes Codex Threads. A Client is safe for concurrent use.
type Client struct {
	executable  string
	environment []string
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

	return &Client{
		executable:  resolved,
		environment: snapshotEnvironment(options.Env),
	}, nil
}

// StartThread creates a new Thread.
func (c *Client) StartThread(options ThreadOptions) *Thread {
	return &Thread{client: c, options: options}
}

// ResumeThread reconstructs a persisted Thread from its identifier.
func (c *Client) ResumeThread(id string, options ThreadOptions) (*Thread, error) {
	if id == "" {
		return nil, &ValidationError{Field: "id", Err: errors.New("must not be empty")}
	}
	return &Thread{client: c, options: options, id: id}, nil
}

func snapshotEnvironment(environment map[string]string) []string {
	if environment == nil {
		return slices.Clone(os.Environ())
	}
	cloned := maps.Clone(environment)
	keys := slices.Sorted(maps.Keys(cloned))
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+cloned[key])
	}
	return result
}
