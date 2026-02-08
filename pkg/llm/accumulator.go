package llm

// ToolCallAccumulator collects incremental tool call deltas into complete ToolCalls.
type ToolCallAccumulator struct {
	calls    map[int]*ToolCall
	maxIndex int
}

// NewToolCallAccumulator creates a new accumulator.
func NewToolCallAccumulator() *ToolCallAccumulator {
	return &ToolCallAccumulator{calls: make(map[int]*ToolCall)}
}

// AddDelta merges an incremental tool call delta.
func (a *ToolCallAccumulator) AddDelta(delta ToolCall) {
	idx := delta.Index
	if idx > a.maxIndex {
		a.maxIndex = idx
	}
	existing, ok := a.calls[idx]
	if !ok {
		a.calls[idx] = &ToolCall{
			ID:       delta.ID,
			Type:     delta.Type,
			Function: FunctionCall{Name: delta.Function.Name},
		}
		existing = a.calls[idx]
	}
	// ID and Name only arrive on the first delta for this index
	if delta.ID != "" {
		existing.ID = delta.ID
	}
	if delta.Function.Name != "" {
		existing.Function.Name = delta.Function.Name
	}
	// Arguments are always appended
	existing.Function.Arguments += delta.Function.Arguments
}

// Complete returns all accumulated tool calls in index order.
func (a *ToolCallAccumulator) Complete() []ToolCall {
	result := make([]ToolCall, 0, len(a.calls))
	for i := 0; i <= a.maxIndex; i++ {
		if call, ok := a.calls[i]; ok {
			result = append(result, *call)
		}
	}
	return result
}
