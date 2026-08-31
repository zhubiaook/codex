# Use the fork module path for the Go SDK

The Go SDK will live in `sdk/go`, declare the module path `github.com/zhubiaook/codex/sdk/go`, and expose its primary API from package `codex`. Using the fork-owned path makes published imports resolve to the repository that owns this SDK and avoids falsely presenting the fork as an upstream OpenAI module, while accepting that a future move to another repository path would be a breaking change for consumers.
