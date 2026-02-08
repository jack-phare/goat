package tools

import (
	"context"
	"fmt"
	"sync"
)

// ConfigStore provides runtime configuration get/set.
type ConfigStore interface {
	Get(key string) (any, bool)
	Set(key string, value any) error
}

// InMemoryConfigStore is a simple in-memory configuration store.
type InMemoryConfigStore struct {
	mu   sync.RWMutex
	data map[string]any
}

// NewInMemoryConfigStore creates a new InMemoryConfigStore.
func NewInMemoryConfigStore() *InMemoryConfigStore {
	return &InMemoryConfigStore{data: make(map[string]any)}
}

func (s *InMemoryConfigStore) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *InMemoryConfigStore) Set(key string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

// ConfigTool provides runtime configuration get/set.
type ConfigTool struct {
	Store ConfigStore
}

func (c *ConfigTool) Name() string { return "Config" }

func (c *ConfigTool) Description() string {
	return "Gets or sets runtime configuration values."
}

func (c *ConfigTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"setting": map[string]any{
				"type":        "string",
				"description": "The configuration key to get or set",
			},
			"value": map[string]any{
				"description": "The value to set (omit to read current value)",
			},
		},
		"required": []string{"setting"},
	}
}

func (c *ConfigTool) SideEffect() SideEffectType { return SideEffectNone }

func (c *ConfigTool) Execute(_ context.Context, input map[string]any) (ToolOutput, error) {
	if c.Store == nil {
		return ToolOutput{Content: "Error: config store not configured", IsError: true}, nil
	}

	setting, ok := input["setting"].(string)
	if !ok || setting == "" {
		return ToolOutput{Content: "Error: setting is required", IsError: true}, nil
	}

	// If value is present, set it
	if value, hasValue := input["value"]; hasValue {
		if err := c.Store.Set(setting, value); err != nil {
			return ToolOutput{
				Content: fmt.Sprintf("Error setting %s: %s", setting, err),
				IsError: true,
			}, nil
		}
		return ToolOutput{Content: fmt.Sprintf("%s set to %v", setting, value)}, nil
	}

	// Otherwise, get it
	value, exists := c.Store.Get(setting)
	if !exists {
		return ToolOutput{
			Content: fmt.Sprintf("Error: setting %q not found", setting),
			IsError: true,
		}, nil
	}

	return ToolOutput{Content: fmt.Sprintf("%s = %v", setting, value)}, nil
}
