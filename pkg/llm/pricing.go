package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// modelInfoResponse is the top-level response from LiteLLM /model/info.
type modelInfoResponse struct {
	Data []modelInfoEntry `json:"data"`
}

// modelInfoEntry is a single model entry in the /model/info response.
type modelInfoEntry struct {
	ModelName string          `json:"model_name"`
	ModelInfo modelInfoDetail `json:"model_info"`
}

// modelInfoDetail holds the pricing and capability fields from LiteLLM /model/info.
type modelInfoDetail struct {
	InputCostPerToken           *float64 `json:"input_cost_per_token"`
	OutputCostPerToken          *float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost     *float64 `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost *float64 `json:"cache_creation_input_token_cost"`
	MaxInputTokens              *int     `json:"max_input_tokens"`
	MaxOutputTokens             *int     `json:"max_output_tokens"`
	SupportsToolUse             *bool    `json:"supports_function_calling"`
}

// FetchPricingResult holds data extracted from the LiteLLM /model/info endpoint.
type FetchPricingResult struct {
	ContextLimits   map[string]int              // model → max input tokens
	MaxOutputTokens map[string]int              // model → max output tokens
	Capabilities    map[string]ModelCapabilities // model → capabilities
}

// FetchPricing calls the LiteLLM /model/info endpoint and merges pricing
// data into DefaultPricing. It also returns context limit and capability data
// for the caller to wire into context/capabilities packages (avoiding import cycles).
// Errors are non-fatal — the caller can log and continue with hardcoded pricing.
func FetchPricing(ctx context.Context, baseURL, apiKey string) (*FetchPricingResult, error) {
	infoURL := deriveModelInfoURL(baseURL)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, infoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", infoURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %d", infoURL, resp.StatusCode)
	}

	var info modelInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	result := &FetchPricingResult{
		ContextLimits:   make(map[string]int),
		MaxOutputTokens: make(map[string]int),
		Capabilities:    make(map[string]ModelCapabilities),
	}

	merged := 0
	for _, entry := range info.Data {
		if entry.ModelName == "" {
			continue
		}
		p := modelInfoTopricing(entry.ModelInfo)
		if p.InputPerMTok == 0 && p.OutputPerMTok == 0 {
			continue // skip models with no pricing data
		}
		SetPricing(entry.ModelName, p)
		merged++

		// Extract context limits and capabilities
		var caps ModelCapabilities
		if entry.ModelInfo.MaxInputTokens != nil && *entry.ModelInfo.MaxInputTokens > 0 {
			result.ContextLimits[entry.ModelName] = *entry.ModelInfo.MaxInputTokens
			caps.MaxInputTokens = *entry.ModelInfo.MaxInputTokens
		}
		if entry.ModelInfo.MaxOutputTokens != nil && *entry.ModelInfo.MaxOutputTokens > 0 {
			result.MaxOutputTokens[entry.ModelName] = *entry.ModelInfo.MaxOutputTokens
			caps.MaxOutputTokens = *entry.ModelInfo.MaxOutputTokens
		}
		if entry.ModelInfo.SupportsToolUse != nil {
			caps.SupportsToolUse = *entry.ModelInfo.SupportsToolUse
		}
		if caps.MaxInputTokens > 0 || caps.MaxOutputTokens > 0 || caps.SupportsToolUse {
			result.Capabilities[entry.ModelName] = caps
			SetCapabilities(entry.ModelName, caps)
		}
	}

	if merged == 0 {
		return nil, fmt.Errorf("no pricing data found in response (%d models)", len(info.Data))
	}
	return result, nil
}

// deriveModelInfoURL strips /v1 suffix (if present) and appends /model/info.
func deriveModelInfoURL(baseURL string) string {
	u := strings.TrimRight(baseURL, "/")
	u = strings.TrimSuffix(u, "/v1")
	return u + "/model/info"
}

// modelInfoTopricing converts per-token costs to per-million-token costs.
func modelInfoTopricing(d modelInfoDetail) ModelPricing {
	var p ModelPricing
	if d.InputCostPerToken != nil {
		p.InputPerMTok = *d.InputCostPerToken * 1_000_000
	}
	if d.OutputCostPerToken != nil {
		p.OutputPerMTok = *d.OutputCostPerToken * 1_000_000
	}
	if d.CacheReadInputTokenCost != nil {
		p.CacheReadPerMTok = *d.CacheReadInputTokenCost * 1_000_000
	}
	if d.CacheCreationInputTokenCost != nil {
		p.CacheCreatePerMTok = *d.CacheCreationInputTokenCost * 1_000_000
	}
	return p
}
