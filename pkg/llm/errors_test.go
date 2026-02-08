package llm

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		statusCode int
		sdkError   string
		retryable  bool
	}{
		{401, "authentication_failed", false},
		{402, "billing_error", false},
		{403, "billing_error", false},
		{400, "invalid_request", false},
		{422, "invalid_request", false},
		{429, "rate_limit", true},
		{529, "rate_limit", true},
		{500, "server_error", true},
		{502, "server_error", true},
		{503, "server_error", true},
		{418, "unknown", false},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.statusCode), func(t *testing.T) {
			resp := &http.Response{
				StatusCode: tt.statusCode,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("error message")),
			}
			err := classifyError(resp)
			if err.SDKError != tt.sdkError {
				t.Errorf("SDKError = %q, want %q", err.SDKError, tt.sdkError)
			}
			if err.Retryable != tt.retryable {
				t.Errorf("Retryable = %v, want %v", err.Retryable, tt.retryable)
			}
			if err.StatusCode != tt.statusCode {
				t.Errorf("StatusCode = %d, want %d", err.StatusCode, tt.statusCode)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Run("seconds", func(t *testing.T) {
		d := parseRetryAfter("5")
		if d != 5*1e9 { // 5 seconds in nanoseconds
			t.Errorf("parseRetryAfter(\"5\") = %v, want 5s", d)
		}
	})

	t.Run("empty", func(t *testing.T) {
		d := parseRetryAfter("")
		if d != 0 {
			t.Errorf("parseRetryAfter(\"\") = %v, want 0", d)
		}
	})

	t.Run("zero", func(t *testing.T) {
		d := parseRetryAfter("0")
		if d != 0 {
			t.Errorf("parseRetryAfter(\"0\") = %v, want 0", d)
		}
	})
}

func TestLLMErrorString(t *testing.T) {
	err := &LLMError{
		StatusCode: 429,
		SDKError:   "rate_limit",
		Message:    "too many requests",
	}
	expected := "llm: rate_limit (HTTP 429): too many requests"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestErrMaxRetriesExceededString(t *testing.T) {
	err := &ErrMaxRetriesExceeded{Attempts: 4, LastStatus: 429}
	expected := "llm: max retries exceeded (4 attempts, last HTTP 429)"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}
