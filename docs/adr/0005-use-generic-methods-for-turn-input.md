# Use generic methods for turn input

`Thread.Run` and `Thread.RunStreamed` will use Go 1.27 generic methods constrained to plain strings and `StructuredInput`, providing compile-time equivalents of the TypeScript SDK's two input forms without accepting `any`. Consumers that need mockable interfaces must wrap the SDK at their application boundary because Go 1.27 does not permit generic interface methods; this trade-off is accepted in favor of a concise, type-safe public API.
