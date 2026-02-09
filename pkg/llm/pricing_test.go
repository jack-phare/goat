package llm

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jg-phare/goat/pkg/types"
)

func TestDeriveModelInfoURL(t *testing.T) {
	tests := []struct {
		baseURL string
		want    string
	}{
		{"http://localhost:4000/v1", "http://localhost:4000/model/info"},
		{"http://localhost:4000/v1/", "http://localhost:4000/model/info"},
		{"http://localhost:4000", "http://localhost:4000/model/info"},
		{"http://localhost:4000/", "http://localhost:4000/model/info"},
		{"https://proxy.example.com/v1", "https://proxy.example.com/model/info"},
	}
	for _, tt := range tests {
		got := deriveModelInfoURL(tt.baseURL)
		if got != tt.want {
			t.Errorf("deriveModelInfoURL(%q) = %q, want %q", tt.baseURL, got, tt.want)
		}
	}
}

func TestModelInfoTopricing(t *testing.T) {
	inputCost := 1.65e-07
	outputCost := 6.6e-07
	cacheRead := 8.25e-08
	cacheCreate := 2.0625e-07

	p := modelInfoTopricing(modelInfoDetail{
		InputCostPerToken:           &inputCost,
		OutputCostPerToken:          &outputCost,
		CacheReadInputTokenCost:     &cacheRead,
		CacheCreationInputTokenCost: &cacheCreate,
	})

	assertClose(t, "InputPerMTok", p.InputPerMTok, 0.165)
	assertClose(t, "OutputPerMTok", p.OutputPerMTok, 0.66)
	assertClose(t, "CacheReadPerMTok", p.CacheReadPerMTok, 0.0825)
	assertClose(t, "CacheCreatePerMTok", p.CacheCreatePerMTok, 0.20625)
}

func TestModelInfoTopricingNilFields(t *testing.T) {
	// Only input/output set, cache fields nil.
	inputCost := 1e-06
	outputCost := 2e-06
	p := modelInfoTopricing(modelInfoDetail{
		InputCostPerToken:  &inputCost,
		OutputCostPerToken: &outputCost,
	})
	assertClose(t, "InputPerMTok", p.InputPerMTok, 1.0)
	assertClose(t, "OutputPerMTok", p.OutputPerMTok, 2.0)
	if p.CacheReadPerMTok != 0 {
		t.Errorf("CacheReadPerMTok = %f, want 0", p.CacheReadPerMTok)
	}
	if p.CacheCreatePerMTok != 0 {
		t.Errorf("CacheCreatePerMTok = %f, want 0", p.CacheCreatePerMTok)
	}
}

// litellmModelInfoResponse is a realistic /model/info response body.
const litellmModelInfoResponse = `{
  "data": [
    {
      "model_name": "gpt-5-nano",
      "model_info": {
        "input_cost_per_token": 1.1e-07,
        "output_cost_per_token": 4.4e-07,
        "cache_read_input_token_cost": 5.5e-08,
        "cache_creation_input_token_cost": 1.375e-07
      }
    },
    {
      "model_name": "gpt-5-mini",
      "model_info": {
        "input_cost_per_token": 3e-07,
        "output_cost_per_token": 1.2e-06
      }
    },
    {
      "model_name": "empty-pricing",
      "model_info": {}
    }
  ]
}`

func TestFetchPricing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/model/info" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(litellmModelInfoResponse))
	}))
	defer srv.Close()

	// Save original pricing to restore after test.
	origPricing := snapshotPricing()
	defer restorePricing(origPricing)

	err := FetchPricing(context.Background(), srv.URL+"/v1", "test-key")
	if err != nil {
		t.Fatalf("FetchPricing returned error: %v", err)
	}

	// Verify gpt-5-nano was merged.
	p, ok := GetPricing("gpt-5-nano")
	if !ok {
		t.Fatal("gpt-5-nano not found in pricing")
	}
	assertClose(t, "gpt-5-nano InputPerMTok", p.InputPerMTok, 0.11)
	assertClose(t, "gpt-5-nano OutputPerMTok", p.OutputPerMTok, 0.44)
	assertClose(t, "gpt-5-nano CacheReadPerMTok", p.CacheReadPerMTok, 0.055)
	assertClose(t, "gpt-5-nano CacheCreatePerMTok", p.CacheCreatePerMTok, 0.1375)

	// Verify gpt-5-mini was merged (no cache pricing).
	p, ok = GetPricing("gpt-5-mini")
	if !ok {
		t.Fatal("gpt-5-mini not found in pricing")
	}
	assertClose(t, "gpt-5-mini InputPerMTok", p.InputPerMTok, 0.3)
	assertClose(t, "gpt-5-mini OutputPerMTok", p.OutputPerMTok, 1.2)

	// Verify empty-pricing was NOT merged (all zeros).
	_, ok = GetPricing("empty-pricing")
	if ok {
		t.Error("empty-pricing should not have been merged")
	}

	// Verify hardcoded Claude pricing still exists.
	p, ok = GetPricing("claude-opus-4-5-20250514")
	if !ok {
		t.Fatal("claude-opus-4-5-20250514 pricing clobbered")
	}
	assertClose(t, "claude-opus input", p.InputPerMTok, 15.0)
}

func TestFetchPricingCalculateCost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(litellmModelInfoResponse))
	}))
	defer srv.Close()

	origPricing := snapshotPricing()
	defer restorePricing(origPricing)

	FetchPricing(context.Background(), srv.URL, "")

	// gpt-5-nano: 1000 input * 0.11/1M + 500 output * 0.44/1M = 0.00011 + 0.00022 = 0.00033
	cost := CalculateCost("gpt-5-nano", types.BetaUsage{InputTokens: 1000, OutputTokens: 500})
	assertClose(t, "gpt-5-nano cost", cost, 0.00033)
}

func TestFetchPricingAuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := FetchPricing(context.Background(), srv.URL+"/v1", "bad-key")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestFetchPricingServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := FetchPricing(context.Background(), srv.URL, "")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestFetchPricingTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := FetchPricing(ctx, "http://localhost:1/v1", "")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestFetchPricingInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	err := FetchPricing(context.Background(), srv.URL, "")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFetchPricingEmptyData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data": []}`))
	}))
	defer srv.Close()

	err := FetchPricing(context.Background(), srv.URL, "")
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

// --- helpers ---

func assertClose(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %g, want %g", name, got, want)
	}
}

func snapshotPricing() map[string]ModelPricing {
	pricingMu.RLock()
	defer pricingMu.RUnlock()
	snap := make(map[string]ModelPricing, len(DefaultPricing))
	for k, v := range DefaultPricing {
		snap[k] = v
	}
	return snap
}

func restorePricing(snap map[string]ModelPricing) {
	pricingMu.Lock()
	defer pricingMu.Unlock()
	DefaultPricing = snap
}
