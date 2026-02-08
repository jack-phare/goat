package llm

import (
	"context"
	"math"
	"math/rand"
	"net/http"
	"time"
)

// doWithRetry executes makeRequest with retry logic for transient failures.
func doWithRetry(ctx context.Context, config RetryConfig, makeRequest func(ctx context.Context) (*http.Response, error)) (*http.Response, error) {
	var lastStatus int

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			backoff := float64(config.InitialBackoff) * math.Pow(config.BackoffFactor, float64(attempt-1))
			if backoff > float64(config.MaxBackoff) {
				backoff = float64(config.MaxBackoff)
			}
			jitter := backoff * config.JitterFraction * rand.Float64()
			sleepDur := time.Duration(backoff + jitter)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(sleepDur):
			}
		}

		resp, err := makeRequest(ctx)
		if err != nil {
			// Network-level error: if context cancelled, return immediately
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			// Network errors are retryable
			lastStatus = 0
			continue
		}

		if resp.StatusCode == 200 {
			return resp, nil
		}

		lastStatus = resp.StatusCode

		// Check Retry-After header (especially for 429)
		if retryAfter := parseRetryAfter(resp.Header.Get("Retry-After")); retryAfter > 0 {
			resp.Body.Close()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryAfter):
			}
			// Don't return — the loop will make another attempt via continue
			continue
		}

		if !isRetryable(resp.StatusCode, config.RetryableStatuses) {
			return resp, nil // caller will classify the error
		}

		resp.Body.Close()
	}

	return nil, &ErrMaxRetriesExceeded{
		Attempts:   config.MaxRetries + 1,
		LastStatus: lastStatus,
	}
}
