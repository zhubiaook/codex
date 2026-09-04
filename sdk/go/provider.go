package codex

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const responsesProviderID = "go_sdk_provider"

// ResponsesProvider configures an OpenAI Responses API-compatible model service.
type ResponsesProvider struct {
	// BaseURL is the API root immediately before the final /responses route.
	BaseURL string
	// Model is the model used when a Thread does not select an override.
	Model string
	// Authentication selects bearer-token or unauthenticated access.
	Authentication ProviderAuthentication
	// Name identifies the provider in diagnostics. When empty, the URL host is used.
	Name string
	// SupportsWebSockets enables the provider's WebSocket transport capability.
	SupportsWebSockets bool
}

// ProviderAuthentication is a closed authentication choice for a ResponsesProvider.
// Construct values with BearerToken or NoAuthentication.
type ProviderAuthentication struct {
	kind        providerAuthenticationKind
	bearerToken string
}

type providerAuthenticationKind uint8

const (
	providerAuthenticationUnset providerAuthenticationKind = iota
	providerAuthenticationBearerToken
	providerAuthenticationNone
)

// BearerToken authenticates a ResponsesProvider with the supplied token.
func BearerToken(token string) ProviderAuthentication {
	return ProviderAuthentication{
		kind:        providerAuthenticationBearerToken,
		bearerToken: strings.Clone(token),
	}
}

// NoAuthentication explicitly configures unauthenticated provider access.
func NoAuthentication() ProviderAuthentication {
	return ProviderAuthentication{kind: providerAuthenticationNone}
}

// ExperimentalClientOptions exposes unmodeled Codex CLI configuration without
// compatibility guarantees.
type ExperimentalClientOptions struct {
	// ConfigOverrides contains ordered raw key=value overrides.
	ConfigOverrides []string
}

type normalizedResponsesProvider struct {
	model     string
	apiKey    string
	overrides []string
}

func normalizeResponsesProvider(provider ResponsesProvider) (normalizedResponsesProvider, error) {
	baseURL, host, err := normalizeProviderBaseURL(provider.BaseURL)
	if err != nil {
		return normalizedResponsesProvider{}, err
	}
	if err := validateRequiredConfigString("provider.model", provider.Model); err != nil {
		return normalizedResponsesProvider{}, err
	}
	name := provider.Name
	if name == "" {
		name = host
	} else if err := validateConfigString("provider.name", name); err != nil {
		return normalizedResponsesProvider{}, err
	}

	apiKey := ""
	providerPrefix := "model_providers." + responsesProviderID
	overrides := []string{
		`model_provider="` + responsesProviderID + `"`,
		providerPrefix + ".base_url=" + quotedConfigString(baseURL),
	}
	switch provider.Authentication.kind {
	case providerAuthenticationUnset:
		return normalizedResponsesProvider{}, newValidationError(
			"provider.authentication",
			"must be created with BearerToken or NoAuthentication",
		)
	case providerAuthenticationBearerToken:
		if err := validateRequiredConfigString(
			"provider.authentication.bearerToken",
			provider.Authentication.bearerToken,
		); err != nil {
			return normalizedResponsesProvider{}, err
		}
		apiKey = provider.Authentication.bearerToken
		overrides = append(overrides, providerPrefix+`.env_key="`+apiKeyEnv+`"`)
	case providerAuthenticationNone:
	}
	overrides = append(overrides, providerPrefix+".name="+quotedConfigString(name))
	if provider.SupportsWebSockets {
		overrides = append(overrides, providerPrefix+".supports_websockets=true")
	}
	return normalizedResponsesProvider{
		model:     strings.Clone(provider.Model),
		apiKey:    strings.Clone(apiKey),
		overrides: overrides,
	}, nil
}

func normalizeProviderBaseURL(value string) (string, string, error) {
	if err := validateRequiredConfigString("provider.baseURL", value); err != nil {
		return "", "", err
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", "", &ValidationError{Field: "provider.baseURL", Err: err}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", newValidationError("provider.baseURL", "scheme must be http or https")
	}
	host := parsed.Hostname()
	if host == "" || parsed.Opaque != "" {
		return "", "", newValidationError("provider.baseURL", "must be an absolute URL with a host")
	}
	if parsed.User != nil {
		return "", "", newValidationError("provider.baseURL", "must not contain user information")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return "", "", newValidationError("provider.baseURL", "must not contain a query")
	}
	if parsed.Fragment != "" {
		return "", "", newValidationError("provider.baseURL", "must not contain a fragment")
	}
	normalizedPath := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(normalizedPath, "/responses") {
		return "", "", newValidationError(
			"provider.baseURL",
			"must identify the API root before the final /responses route",
		)
	}
	return strings.TrimRight(value, "/"), host, nil
}

func validateRequiredConfigString(field string, value string) error {
	if value == "" {
		return newValidationError(field, "must not be empty")
	}
	return validateConfigString(field, value)
}

func validateConfigString(field string, value string) error {
	if strings.TrimSpace(value) != value {
		return newValidationError(field, "must not contain surrounding whitespace")
	}
	return nil
}

func newValidationError(field string, reason string) error {
	return &ValidationError{Field: field, Err: errors.New(reason)}
}

func quotedConfigString(value string) string {
	rendered, _ := json.Marshal(value)
	return string(rendered)
}

func validateExperimentalOverrides(overrides []string) error {
	for index, override := range overrides {
		key, _, ok := strings.Cut(override, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		rootKey := experimentalOverrideRootKey(key)
		if rootKey == "model" || rootKey == "model_provider" || rootKey == "openai_base_url" ||
			rootKey == "model_providers" {
			return newValidationError(
				fmt.Sprintf("experimental.configOverrides[%d]", index),
				fmt.Sprintf("must not override SDK-managed key %q", key),
			)
		}
	}
	return nil
}

func experimentalOverrideRootKey(key string) string {
	root, _, _ := strings.Cut(key, ".")
	root = strings.TrimSpace(root)
	if len(root) < 2 {
		return root
	}
	if root[0] == '\'' && root[len(root)-1] == '\'' {
		return root[1 : len(root)-1]
	}
	if root[0] == '"' && root[len(root)-1] == '"' {
		if unquoted, err := strconv.Unquote(root); err == nil {
			return unquoted
		}
	}
	return root
}
