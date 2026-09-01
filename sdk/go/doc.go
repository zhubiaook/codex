// Package codex embeds the Codex agent in Go applications and workflows.
//
// A Client starts or resumes persisted Threads. Each Thread accepts consecutive
// Turns and exposes either a buffered Turn result or a lazy stream of typed
// Thread Events. Completed Thread Items describe agent messages, reasoning,
// command execution, file changes, MCP calls, web searches, plans, and errors.
//
// The package runs an installed Codex CLI for each Turn. It owns JSONL decoding,
// process cleanup, output-schema files, configuration precedence, and Thread
// resumption. Clients are safe for concurrent use. A single Thread accepts one
// active Turn at a time, while separate Threads may run concurrently.
package codex
