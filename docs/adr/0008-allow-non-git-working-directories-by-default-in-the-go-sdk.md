# Allow non-Git working directories by default in the Go SDK

The Go SDK will allow Threads to run outside Git repositories by default because embedded applications commonly use temporary or application-managed working directories, and repository membership is unrelated to model-provider configuration. Callers that require the existing trust check can opt in with a positively named `RequireGitRepository` Thread option. This reverses the CLI-oriented default and removes the double-negative `SkipGitRepoCheck` option so an otherwise empty Thread configuration is a valid SDK starting point.
