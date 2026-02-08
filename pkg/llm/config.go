package llm

import (
	"net/http"
	"time"
)

// ClientConfig holds LLM client configuration.
type ClientConfig struct {
	BaseURL            string            // LiteLLM proxy URL, e.g. "http://localhost:4000/v1"
	APIKey             string            // LiteLLM virtual key or Anthropic API key
	Model              string            // Default model, e.g. "anthropic/claude-opus-4-5-20250514"
	MaxTokens          int               // Default max_tokens for responses (16384)
	MaxThinkingTokens  int               // Budget tokens for extended thinking (0 = disabled)
	Betas              []string          // Beta feature flags, e.g. ["context-1m-2025-08-07"]
	Headers            map[string]string // Additional HTTP headers
	HTTPClient         *http.Client      // Custom HTTP client (for timeouts, TLS, proxies)
	Retry              RetryConfig
	CostTracker        *CostTracker // Optional cost accumulation across requests
}

// RetryConfig controls retry behavior for transient failures.
type RetryConfig struct {
	MaxRetries        int           // Max retry attempts (default: 3)
	InitialBackoff    time.Duration // Initial backoff (default: 1s)
	MaxBackoff        time.Duration // Max backoff cap (default: 30s)
	BackoffFactor     float64       // Multiplier per retry (default: 2.0)
	JitterFraction    float64       // Random jitter as fraction of backoff (default: 0.1)
	RetryableStatuses []int         // HTTP codes to retry (default: 429, 529, 500, 502, 503)
}

// DefaultRetryConfig returns sensible retry defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        3,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffFactor:     2.0,
		JitterFraction:    0.1,
		RetryableStatuses: []int{429, 529, 500, 502, 503},
	}
}
