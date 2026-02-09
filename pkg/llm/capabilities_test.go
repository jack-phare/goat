package llm

import (
	"sync"
	"testing"
)

func TestGetCapabilities_Known(t *testing.T) {
	caps, ok := GetCapabilities("claude-sonnet-4-5-20250929")
	if !ok {
		t.Fatal("expected to find claude-sonnet capabilities")
	}
	if !caps.SupportsToolUse {
		t.Error("expected SupportsToolUse=true for sonnet")
	}
	if !caps.SupportsThinking {
		t.Error("expected SupportsThinking=true for sonnet")
	}
	if caps.MaxInputTokens != 200_000 {
		t.Errorf("MaxInputTokens = %d, want 200000", caps.MaxInputTokens)
	}
}

func TestGetCapabilities_Unknown(t *testing.T) {
	_, ok := GetCapabilities("unknown-model")
	if ok {
		t.Error("expected unknown model to not have capabilities")
	}
}

func TestSetCapabilities(t *testing.T) {
	SetCapabilities("gpt-5-nano-test", ModelCapabilities{
		SupportsToolUse: true,
		MaxInputTokens:  128000,
		MaxOutputTokens: 16384,
	})
	defer func() {
		capabilityMu.Lock()
		delete(modelCaps, "gpt-5-nano-test")
		capabilityMu.Unlock()
	}()

	caps, ok := GetCapabilities("gpt-5-nano-test")
	if !ok {
		t.Fatal("expected to find gpt-5-nano-test capabilities")
	}
	if !caps.SupportsToolUse {
		t.Error("expected SupportsToolUse=true")
	}
	if caps.MaxInputTokens != 128000 {
		t.Errorf("MaxInputTokens = %d, want 128000", caps.MaxInputTokens)
	}
}

func TestCapabilities_ConcurrentSafe(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			SetCapabilities("concurrent-cap-test", ModelCapabilities{SupportsToolUse: true})
		}()
		go func() {
			defer wg.Done()
			_, _ = GetCapabilities("concurrent-cap-test")
		}()
	}
	wg.Wait()

	capabilityMu.Lock()
	delete(modelCaps, "concurrent-cap-test")
	capabilityMu.Unlock()
}
