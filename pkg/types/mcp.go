package types

// McpServerConfig is the union type for MCP server connection configuration.
type McpServerConfig struct {
	Type string `json:"type"` // "stdio"|"sse"|"http"|"sdk"|"claudeai-proxy"

	// stdio
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// sse/http
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`

	// sdk
	Name string `json:"name,omitempty"`

	// claudeai-proxy
	ID string `json:"id,omitempty"`
}

// PluginConfig describes a plugin to load.
type PluginConfig struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// SandboxSettings controls command execution isolation.
type SandboxSettings struct {
	Enabled                  bool                  `json:"enabled"`
	AutoAllowBashIfSandboxed bool                  `json:"autoAllowBashIfSandboxed,omitempty"`
	Network                  *SandboxNetworkConfig `json:"network,omitempty"`
	IgnoreViolations         []string              `json:"ignoreViolations,omitempty"`
}

// SandboxNetworkConfig controls network access within a sandbox.
type SandboxNetworkConfig struct {
	AllowLocalBinding bool     `json:"allowLocalBinding,omitempty"`
	AllowUnixSockets  []string `json:"allowUnixSockets,omitempty"`
}
