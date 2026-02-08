package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// Client is the LLM inference client. All methods are safe for concurrent use.
type Client interface {
	// Complete sends a streaming completion request and returns a Stream.
	Complete(ctx context.Context, req *CompletionRequest) (*Stream, error)

	// Model returns the configured default model string.
	Model() string

	// SetModel changes the default model for subsequent requests.
	SetModel(model string)
}

// httpClient implements the Client interface.
type httpClient struct {
	config     ClientConfig
	httpClient *http.Client
	mu         sync.RWMutex
}

// NewClient creates a new LLM client with the given configuration.
func NewClient(cfg ClientConfig) Client {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 16384
	}
	if cfg.Retry.MaxRetries == 0 && cfg.Retry.InitialBackoff == 0 {
		cfg.Retry = DefaultRetryConfig()
	}

	return &httpClient{
		config:     cfg,
		httpClient: cfg.HTTPClient,
	}
}

// Complete sends a streaming completion request and returns a Stream.
func (c *httpClient) Complete(ctx context.Context, req *CompletionRequest) (*Stream, error) {
	// Ensure streaming is enabled
	req.Stream = true
	if req.StreamOptions == nil {
		req.StreamOptions = &StreamOptions{IncludeUsage: true}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("llm: marshal request: %w", err)
	}

	url := c.config.BaseURL + "/chat/completions"

	resp, err := doWithRetry(ctx, c.config.Retry, func(ctx context.Context) (*http.Response, error) {
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}

		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")

		if c.config.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+c.config.APIKey)
		}

		for k, v := range c.config.Headers {
			httpReq.Header.Set(k, v)
		}

		return c.httpClient.Do(httpReq)
	})

	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		llmErr := classifyError(resp)
		resp.Body.Close()
		return nil, llmErr
	}

	// Create a cancellable context for the stream
	streamCtx, cancel := context.WithCancel(ctx)
	events := ParseSSEStream(streamCtx, resp.Body)

	return NewStream(events, resp.Body, cancel), nil
}

// Model returns the configured default model string.
func (c *httpClient) Model() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config.Model
}

// SetModel changes the default model for subsequent requests.
func (c *httpClient) SetModel(model string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.Model = model
}
