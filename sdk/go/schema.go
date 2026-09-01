package codex

import (
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"os"
	"path/filepath"
)

func createOutputSchemaFile(schema json.RawMessage) (string, func(), error) {
	if schema == nil {
		return "", func() {}, nil
	}
	value := jsontext.Value(schema)
	if !value.IsValid() || value.Kind() != '{' {
		return "", func() {}, &ValidationError{
			Field: "output schema",
			Err:   errors.New("must be a valid top-level JSON object"),
		}
	}
	directory, err := os.MkdirTemp("", "codex-output-schema-")
	if err != nil {
		return "", func() {}, &ValidationError{Field: "output schema", Err: err}
	}
	cleanup := func() {
		_ = os.RemoveAll(directory)
	}
	path := filepath.Join(directory, "schema.json")
	if err := os.WriteFile(path, schema, 0o600); err != nil {
		cleanup()
		return "", func() {}, &ValidationError{Field: "output schema", Err: err}
	}
	return path, cleanup, nil
}
