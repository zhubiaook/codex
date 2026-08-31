# Model wire unions with forward-compatible fallbacks

The Go SDK will represent thread events and thread items as SDK-sealed interfaces with exported concrete types for every known discriminator. Unknown discriminators will decode into `UnknownEvent` or `UnknownItem` values that preserve the discriminator and raw JSON payload, preventing additive Codex CLI protocol changes from failing otherwise valid turns; malformed JSON and invalid payloads for known discriminators will still return errors.
