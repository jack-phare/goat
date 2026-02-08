package mcp

// ServerStatus is an external view of a server connection's state.
type ServerStatus struct {
	Name       string           `json:"name"`
	Status     ConnectionStatus `json:"status"`
	ServerInfo *ServerInfo      `json:"serverInfo,omitempty"`
	Error      string           `json:"error,omitempty"`
	Tools      []ToolInfo       `json:"tools,omitempty"`
}

// SetServersResult reports what changed after a SetServers call.
type SetServersResult struct {
	Added   []string          `json:"added,omitempty"`
	Removed []string          `json:"removed,omitempty"`
	Errors  map[string]string `json:"errors,omitempty"`
}
