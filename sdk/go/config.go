package codex

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

func serializeConfig(config map[string]any) ([]string, error) {
	if config == nil {
		return nil, nil
	}
	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	overrides := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			return nil, configValidationError("config", "key must not be empty")
		}
		if err := flattenConfigValue(config[key], key, &overrides); err != nil {
			return nil, err
		}
	}
	return overrides, nil
}

func flattenConfigValue(value any, path string, overrides *[]string) error {
	if object, ok := value.(map[string]any); ok {
		if len(object) == 0 {
			*overrides = append(*overrides, path+"={}")
			return nil
		}
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			if key == "" {
				return configValidationError(path, "key must not be empty")
			}
			if err := flattenConfigValue(object[key], path+"."+key, overrides); err != nil {
				return err
			}
		}
		return nil
	}
	rendered, err := renderTOMLValue(value, path)
	if err != nil {
		return err
	}
	*overrides = append(*overrides, path+"="+rendered)
	return nil
}

func renderTOMLValue(value any, path string) (string, error) {
	switch value := value.(type) {
	case string:
		encoded, err := json.Marshal(value)
		if err != nil {
			return "", configValidationError(path, "string cannot be encoded")
		}
		return string(encoded), nil
	case bool:
		return strconv.FormatBool(value), nil
	case int:
		return strconv.FormatInt(int64(value), 10), nil
	case int8:
		return strconv.FormatInt(int64(value), 10), nil
	case int16:
		return strconv.FormatInt(int64(value), 10), nil
	case int32:
		return strconv.FormatInt(int64(value), 10), nil
	case int64:
		return strconv.FormatInt(value, 10), nil
	case uint:
		return strconv.FormatUint(uint64(value), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(value), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(value), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(value), 10), nil
	case uint64:
		return strconv.FormatUint(value, 10), nil
	case float32:
		return renderFloat(float64(value), 32, path)
	case float64:
		return renderFloat(value, 64, path)
	case []any:
		parts := make([]string, len(value))
		for index, child := range value {
			rendered, err := renderTOMLValue(child, fmt.Sprintf("%s[%d]", path, index))
			if err != nil {
				return "", err
			}
			parts[index] = rendered
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			if key == "" {
				return "", configValidationError(path, "key must not be empty")
			}
			rendered, err := renderTOMLValue(value[key], path+"."+key)
			if err != nil {
				return "", err
			}
			parts = append(parts, formatTOMLKey(key)+" = "+rendered)
		}
		return "{" + strings.Join(parts, ", ") + "}", nil
	case nil:
		return "", configValidationError(path, "value must not be nil")
	default:
		return "", configValidationError(path, "unsupported value type "+reflect.TypeOf(value).String())
	}
}

func renderFloat(value float64, bits int, path string) (string, error) {
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return "", configValidationError(path, "number must be finite")
	}
	return strconv.FormatFloat(value, 'g', -1, bits), nil
}

func formatTOMLKey(key string) string {
	if key != "" && strings.IndexFunc(key, func(character rune) bool {
		return !((character >= 'A' && character <= 'Z') ||
			(character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') || character == '_' || character == '-')
	}) == -1 {
		return key
	}
	encoded, _ := json.Marshal(key)
	return string(encoded)
}

func configValidationError(path string, reason string) error {
	return &ValidationError{Field: path, Err: errors.New(reason)}
}
