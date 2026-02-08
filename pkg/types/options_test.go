package types

import (
	"encoding/json"
	"testing"
)

func TestSystemPromptConfig_MarshalJSON_Raw(t *testing.T) {
	cfg := SystemPromptConfig{Raw: "You are helpful."}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Should be a plain JSON string
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("expected plain string, got: %s", data)
	}
	if s != "You are helpful." {
		t.Errorf("got %q", s)
	}
}

func TestSystemPromptConfig_MarshalJSON_Preset(t *testing.T) {
	cfg := SystemPromptConfig{Preset: "claude_code"}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("expected object, got: %s", data)
	}
	if m["type"] != "preset" {
		t.Errorf("type = %v", m["type"])
	}
	if m["preset"] != "claude_code" {
		t.Errorf("preset = %v", m["preset"])
	}
}

func TestSystemPromptConfig_MarshalJSON_PresetWithAppend(t *testing.T) {
	cfg := SystemPromptConfig{Preset: "claude_code", Append: "Be concise."}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	json.Unmarshal(data, &m)
	if m["append"] != "Be concise." {
		t.Errorf("append = %v", m["append"])
	}
}

func TestQueryOptions_JSONRoundTrip(t *testing.T) {
	maxTurns := 10
	opts := QueryOptions{
		Model:    "claude-opus",
		CWD:      "/home",
		MaxTurns: &maxTurns,
		Betas:    []string{"beta-1"},
	}

	data, err := json.Marshal(opts)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded QueryOptions
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Model != "claude-opus" {
		t.Errorf("Model = %q", decoded.Model)
	}
	if decoded.CWD != "/home" {
		t.Errorf("CWD = %q", decoded.CWD)
	}
	if decoded.MaxTurns == nil || *decoded.MaxTurns != 10 {
		t.Errorf("MaxTurns = %v", decoded.MaxTurns)
	}
	if len(decoded.Betas) != 1 || decoded.Betas[0] != "beta-1" {
		t.Errorf("Betas = %v", decoded.Betas)
	}
}
