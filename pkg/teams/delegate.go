package teams

import "sync"

// DelegateModeTools lists the tools available in delegate (lead agent) mode.
// TaskCreate/TaskUpdate/TaskList are forward-declared; implementations pending.
var DelegateModeTools = []string{
	"TeamCreate",
	"SendMessage",
	"TeamDelete",
	"TaskCreate",
	"TaskUpdate",
	"TaskList",
	"AskUserQuestion",
}

// DelegateModeState tracks whether delegate mode is active.
type DelegateModeState struct {
	mu     sync.RWMutex
	active bool
}

// Enable activates delegate mode and returns the tool whitelist.
func (d *DelegateModeState) Enable() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.active = true
	return DelegateModeTools
}

// Disable deactivates delegate mode.
func (d *DelegateModeState) Disable() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.active = false
}

// IsActive returns whether delegate mode is currently active.
func (d *DelegateModeState) IsActive() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.active
}

// FilterTools returns only the tools that are allowed in delegate mode.
// If delegate mode is not active, returns all tool names unchanged.
func (d *DelegateModeState) FilterTools(toolNames []string) []string {
	if !d.IsActive() {
		return toolNames
	}

	allowed := make(map[string]bool, len(DelegateModeTools))
	for _, t := range DelegateModeTools {
		allowed[t] = true
	}

	var filtered []string
	for _, name := range toolNames {
		if allowed[name] {
			filtered = append(filtered, name)
		}
	}
	return filtered
}
