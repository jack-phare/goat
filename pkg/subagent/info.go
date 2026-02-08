package subagent

// AgentInfo provides summary information about an agent definition for UI display.
type AgentInfo struct {
	Name        string
	Description string
	Model       string
	Source      AgentSource
	Tools       []string
	IsActive    bool
	FilePath    string
}

// ListAgentInfo returns info about all registered agent definitions.
func (m *Manager) ListAgentInfo() []AgentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Build set of active agent types
	activeTypes := make(map[string]bool)
	for _, ra := range m.active {
		if ra.GetState() == StateRunning {
			activeTypes[ra.Type] = true
		}
	}

	result := make([]AgentInfo, 0, len(m.agents))
	for name, def := range m.agents {
		result = append(result, AgentInfo{
			Name:        name,
			Description: def.Description,
			Model:       def.Model,
			Source:      def.Source,
			Tools:       def.Tools,
			IsActive:    activeTypes[name],
			FilePath:    def.FilePath,
		})
	}
	return result
}
