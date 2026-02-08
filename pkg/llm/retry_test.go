package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetry(t *testing.T) {
	t.Run("retryable codes retried then succeed", func(t *testing.T) {
		var attempt atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := attempt.Add(1)
			if n <= 2 {
				w.WriteHeader(429)
				fmt.Fprint(w, "rate limited")
				return
			}
			w.WriteHeader(200)
			fmt.Fprint(w, "ok")
		}))
		defer srv.Close()

		config := RetryConfig{
			MaxRetries:        3,
			InitialBackoff:    10 * time.Millisecond, // fast for tests
			MaxBackoff:        100 * time.Millisecond,
			BackoffFactor:     2.0,
			JitterFraction:    0.0, // no jitter for deterministic test
			RetryableStatuses: []int{429, 500, 502, 503, 529},
		}

		resp, err := doWithRetry(context.Background(), config, func(ctx context.Context) (*http.Response, error) {
			return http.Get(srv.URL)
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
		}
		if attempt.Load() != 3 {
			t.Errorf("attempts = %d, want 3", attempt.Load())
		}
	})

	t.Run("non-retryable code fails immediately", func(t *testing.T) {
		var attempt atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempt.Add(1)
			w.WriteHeader(401)
			fmt.Fprint(w, "unauthorized")
		}))
		defer srv.Close()

		config := RetryConfig{
			MaxRetries:        3,
			InitialBackoff:    10 * time.Millisecond,
			MaxBackoff:        100 * time.Millisecond,
			BackoffFactor:     2.0,
			JitterFraction:    0.0,
			RetryableStatuses: []int{429, 500},
		}

		resp, err := doWithRetry(context.Background(), config, func(ctx context.Context) (*http.Response, error) {
			return http.Get(srv.URL)
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		// Non-retryable returns the response for caller to classify
		if resp.StatusCode != 401 {
			t.Errorf("StatusCode = %d, want 401", resp.StatusCode)
		}
		if attempt.Load() != 1 {
			t.Errorf("attempts = %d, want 1 (should not retry)", attempt.Load())
		}
	})

	t.Run("max retries exhausted", func(t *testing.T) {
		var attempt atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempt.Add(1)
			w.WriteHeader(500)
			fmt.Fprint(w, "server error")
		}))
		defer srv.Close()

		config := RetryConfig{
			MaxRetries:        2,
			InitialBackoff:    5 * time.Millisecond,
			MaxBackoff:        50 * time.Millisecond,
			BackoffFactor:     2.0,
			JitterFraction:    0.0,
			RetryableStatuses: []int{500},
		}

		_, err := doWithRetry(context.Background(), config, func(ctx context.Context) (*http.Response, error) {
			return http.Get(srv.URL)
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		maxErr, ok := err.(*ErrMaxRetriesExceeded)
		if !ok {
			t.Fatalf("expected *ErrMaxRetriesExceeded, got %T: %v", err, err)
		}
		if maxErr.Attempts != 3 {
			t.Errorf("Attempts = %d, want 3", maxErr.Attempts)
		}
		if maxErr.LastStatus != 500 {
			t.Errorf("LastStatus = %d, want 500", maxErr.LastStatus)
		}
	})

	t.Run("429 respects Retry-After header", func(t *testing.T) {
		var attempt atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := attempt.Add(1)
			if n == 1 {
				w.Header().Set("Retry-After", "1") // 1 second
				w.WriteHeader(429)
				fmt.Fprint(w, "rate limited")
				return
			}
			w.WriteHeader(200)
			fmt.Fprint(w, "ok")
		}))
		defer srv.Close()

		config := RetryConfig{
			MaxRetries:        3,
			InitialBackoff:    10 * time.Millisecond,
			MaxBackoff:        100 * time.Millisecond,
			BackoffFactor:     2.0,
			JitterFraction:    0.0,
			RetryableStatuses: []int{429},
		}

		start := time.Now()
		resp, err := doWithRetry(context.Background(), config, func(ctx context.Context) (*http.Response, error) {
			return http.Get(srv.URL)
		})
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
		}
		// Should have waited at least ~1s for Retry-After
		if elapsed < 800*time.Millisecond {
			t.Errorf("elapsed = %v, expected >= ~1s (Retry-After header)", elapsed)
		}
	})

	t.Run("context cancellation during backoff", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			fmt.Fprint(w, "error")
		}))
		defer srv.Close()

		config := RetryConfig{
			MaxRetries:        10,
			InitialBackoff:    10 * time.Second, // very long backoff
			MaxBackoff:        30 * time.Second,
			BackoffFactor:     2.0,
			JitterFraction:    0.0,
			RetryableStatuses: []int{500},
		}

		ctx, cancel := context.WithCancel(context.Background())
		// Cancel after a short time
		go func() {
			time.Sleep(100 * time.Millisecond)
			cancel()
		}()

		start := time.Now()
		_, err := doWithRetry(ctx, config, func(ctx context.Context) (*http.Response, error) {
			return http.Get(srv.URL)
		})
		elapsed := time.Since(start)

		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
		// Should have returned promptly after cancel, not waited full backoff
		if elapsed > 1*time.Second {
			t.Errorf("elapsed = %v, expected < 1s (should cancel during backoff)", elapsed)
		}
	})
}
