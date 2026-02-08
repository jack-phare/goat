package llm

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// LLMError wraps HTTP-level errors from the LLM API with SDK classification.
type LLMError struct {
	StatusCode int
	SDKError   string // SDKAssistantMessageError value
	Message    string // Error message from response body
	Retryable  bool
	RetryAfter time.Duration // From Retry-After header, if present
}

func (e *LLMError) Error() string {
	return fmt.Sprintf("llm: %s (HTTP %d): %s", e.SDKError, e.StatusCode, e.Message)
}

// ErrMaxRetriesExceeded is returned when all retry attempts are exhausted.
type ErrMaxRetriesExceeded struct {
	Attempts   int
	LastStatus int
}

func (e *ErrMaxRetriesExceeded) Error() string {
	return fmt.Sprintf("llm: max retries exceeded (%d attempts, last HTTP %d)", e.Attempts, e.LastStatus)
}

// classifyError maps an HTTP response to an LLMError with SDK error type.
func classifyError(resp *http.Response) *LLMError {
	bodyBytes, _ := io.ReadAll(resp.Body)
	msg := string(bodyBytes)
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}

	sdkError, retryable := classifyStatus(resp.StatusCode)

	return &LLMError{
		StatusCode: resp.StatusCode,
		SDKError:   sdkError,
		Message:    msg,
		Retryable:  retryable,
		RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After")),
	}
}

// classifyStatus maps HTTP status code to SDK error type and retryability.
func classifyStatus(statusCode int) (sdkError string, retryable bool) {
	switch statusCode {
	case 401:
		return "authentication_failed", false
	case 402:
		return "billing_error", false
	case 403:
		return "billing_error", false
	case 400:
		return "invalid_request", false
	case 422:
		return "invalid_request", false
	case 429:
		return "rate_limit", true
	case 529:
		return "rate_limit", true
	case 500:
		return "server_error", true
	case 502:
		return "server_error", true
	case 503:
		return "server_error", true
	default:
		return "unknown", false
	}
}

// isRetryable checks if a status code should be retried.
func isRetryable(statusCode int, retryableStatuses []int) bool {
	for _, s := range retryableStatuses {
		if statusCode == s {
			return true
		}
	}
	return false
}

// parseRetryAfter parses the Retry-After header value.
// Supports both seconds (integer) and HTTP-date formats.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return 0
	}

	// Try parsing as seconds
	var seconds int
	if _, err := fmt.Sscanf(value, "%d", &seconds); err == nil {
		return time.Duration(seconds) * time.Second
	}

	// Try parsing as HTTP-date
	if t, err := http.ParseTime(value); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
		return 0
	}

	return 0
}
