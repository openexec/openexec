package project

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestActiveAPI_EmptyConfigReturnsZero(t *testing.T) {
	var e ExecutionConfig
	name, baseURL, key, model := e.ActiveAPI()
	if name != "" || baseURL != "" || key != "" || model != "" {
		t.Fatalf("expected all empty, got (%q, %q, %q, %q)", name, baseURL, key, model)
	}
}

func TestActiveAPI_LegacyFieldsOnly(t *testing.T) {
	e := ExecutionConfig{
		APIProvider: "openai_compat",
		APIBaseURL:  "https://api.moonshot.cn/v1",
		APIKey:      "$KIMI_API_KEY",
		APIModel:    "moonshot-v1-128k",
	}
	name, baseURL, key, model := e.ActiveAPI()
	if name != "openai_compat" || baseURL != "https://api.moonshot.cn/v1" ||
		key != "$KIMI_API_KEY" || model != "moonshot-v1-128k" {
		t.Fatalf("legacy fallback wrong: got (%q, %q, %q, %q)", name, baseURL, key, model)
	}
}

func TestActiveAPI_NamedProviderActive(t *testing.T) {
	e := ExecutionConfig{
		Providers: map[string]ProviderConfig{
			"agentics-personal": {
				BaseURL: "https://api.agentics.org.nz/v1",
				APIKey:  "$AGENTICSNZ_API_KEY",
				Model:   "bartowski/google_gemma-4-31B-it-GGUF",
			},
			"vllm-local": {
				BaseURL: "http://localhost:8000/v1",
				APIKey:  "$VLLM_KEY",
				Model:   "llama-3.1-70b",
			},
		},
		ActiveProvider: "vllm-local",
	}
	name, baseURL, key, model := e.ActiveAPI()
	if name != "vllm-local" || baseURL != "http://localhost:8000/v1" ||
		key != "$VLLM_KEY" || model != "llama-3.1-70b" {
		t.Fatalf("active selection wrong: got (%q, %q, %q, %q)", name, baseURL, key, model)
	}
}

func TestActiveAPI_SingleProviderImpliesActive(t *testing.T) {
	e := ExecutionConfig{
		Providers: map[string]ProviderConfig{
			"only-one": {BaseURL: "x", APIKey: "y", Model: "z"},
		},
	}
	name, baseURL, _, _ := e.ActiveAPI()
	if name != "only-one" || baseURL != "x" {
		t.Fatalf("single-entry default-active failed: got name=%q baseURL=%q", name, baseURL)
	}
}

func TestActiveAPI_NamedProvidersWinOverLegacy(t *testing.T) {
	e := ExecutionConfig{
		Providers: map[string]ProviderConfig{
			"agentics-work": {BaseURL: "https://example/v1", APIKey: "$WORK_KEY", Model: "m1"},
		},
		ActiveProvider: "agentics-work",
		// Stale legacy values that should be ignored.
		APIProvider: "openai_compat",
		APIBaseURL:  "https://stale/v1",
		APIKey:      "$STALE",
		APIModel:    "old-model",
	}
	name, baseURL, _, model := e.ActiveAPI()
	if name != "agentics-work" || baseURL != "https://example/v1" || model != "m1" {
		t.Fatalf("named entry must win over legacy: got (%q, %q, %q)", name, baseURL, model)
	}
}

func TestActiveAPI_UnknownActiveFallsBackToLegacy(t *testing.T) {
	// ActiveProvider names a key that isn't in the map and Providers has >1
	// entry (so single-entry default doesn't apply). Falls through to legacy.
	e := ExecutionConfig{
		Providers: map[string]ProviderConfig{
			"a": {BaseURL: "ua"},
			"b": {BaseURL: "ub"},
		},
		ActiveProvider: "ghost",
		APIProvider:    "openai_compat",
		APIBaseURL:     "https://legacy/v1",
		APIKey:         "$LEGACY",
		APIModel:       "legacy-model",
	}
	name, baseURL, key, model := e.ActiveAPI()
	if name != "openai_compat" || baseURL != "https://legacy/v1" ||
		key != "$LEGACY" || model != "legacy-model" {
		t.Fatalf("unknown active should fall back to legacy: got (%q, %q, %q, %q)",
			name, baseURL, key, model)
	}
}

func TestProjectConfig_JSONRoundTrip_NamedProviders(t *testing.T) {
	in := ProjectConfig{
		Name: "demo",
		Execution: ExecutionConfig{
			ExecutorModel:  "sonnet",
			ActiveProvider: "kimi-prod",
			Providers: map[string]ProviderConfig{
				"kimi-prod": {
					BaseURL: "https://api.moonshot.cn/v1",
					APIKey:  "$KIMI_API_KEY",
					Model:   "moonshot-v1-128k",
				},
			},
		},
	}
	data, err := json.Marshal(&in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out ProjectConfig
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in.Execution.Providers, out.Execution.Providers) {
		t.Fatalf("providers round-trip mismatch:\nin:  %#v\nout: %#v",
			in.Execution.Providers, out.Execution.Providers)
	}
	if out.Execution.ActiveProvider != "kimi-prod" {
		t.Fatalf("active_provider lost: %q", out.Execution.ActiveProvider)
	}
}

func TestProjectConfig_JSONRoundTrip_LegacyShapeStillReadable(t *testing.T) {
	// Simulate an existing on-disk config written before the named-providers
	// field was introduced. ActiveAPI() must still resolve correctly.
	const raw = `{
  "name": "legacy-project",
  "execution": {
    "executor_model": "sonnet",
    "api_provider": "agenticsnz",
    "api_base_url": "https://api.agentics.org.nz/v1",
    "api_key": "$AGENTICSNZ_API_KEY",
    "api_model": "bartowski/google_gemma-4-31B-it-GGUF"
  }
}`
	var cfg ProjectConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	name, baseURL, _, model := cfg.Execution.ActiveAPI()
	if name != "agenticsnz" || baseURL != "https://api.agentics.org.nz/v1" ||
		model != "bartowski/google_gemma-4-31B-it-GGUF" {
		t.Fatalf("legacy on-disk config not resolved: (%q, %q, %q)", name, baseURL, model)
	}
}
