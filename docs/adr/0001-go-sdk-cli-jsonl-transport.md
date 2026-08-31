# Use the Codex CLI JSONL transport for the Go SDK

The Go SDK will wrap `codex exec --experimental-json`, send turn input through stdin, consume JSONL events from stdout, and resume conversations by thread ID. This deliberately follows the TypeScript SDK instead of the app-server transport used by the Python SDK, keeping the Go SDK's scope and behavior aligned with its reference implementation at the cost of starting a CLI process for each turn.
