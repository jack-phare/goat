package types

// SystemInitMessage is the first message emitted on session start.
type SystemInitMessage struct {
	BaseMessage
	Type              MessageType     `json:"type"`
	Subtype           SystemSubtype   `json:"subtype"`
	Agents            []string        `json:"agents,omitempty"`
	APIKeySource      string          `json:"apiKeySource"`
	Betas             []string        `json:"betas,omitempty"`
	ClaudeCodeVersion string          `json:"claude_code_version"`
	CWD               string          `json:"cwd"`
	Tools             []string        `json:"tools"`
	McpServers        []McpServerInfo `json:"mcp_servers"`
	Model             string          `json:"model"`
	PermissionMode    PermissionMode  `json:"permissionMode"`
	SlashCommands     []string        `json:"slash_commands"`
	OutputStyle       string          `json:"output_style"`
	Skills            []string        `json:"skills"`
	Plugins           []PluginInfo    `json:"plugins"`
}

func (m SystemInitMessage) GetType() MessageType { return MessageTypeSystem }

// McpServerInfo describes the status of an MCP server connection.
type McpServerInfo struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

// PluginInfo describes a loaded plugin.
type PluginInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// PermissionMode controls tool execution authorization.
type PermissionMode string

const (
	PermissionModeDefault           PermissionMode = "default"
	PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
	PermissionModePlan              PermissionMode = "plan"
	PermissionModeDelegate          PermissionMode = "delegate"
	PermissionModeDontAsk           PermissionMode = "dontAsk"
)

// StatusMessage is emitted during status transitions.
type StatusMessage struct {
	BaseMessage
	Type           MessageType     `json:"type"`
	Subtype        SystemSubtype   `json:"subtype"`
	Status         *string         `json:"status"`
	PermissionMode *PermissionMode `json:"permissionMode,omitempty"`
}

func (m StatusMessage) GetType() MessageType { return MessageTypeSystem }

// CompactBoundaryMessage marks a context compaction boundary.
type CompactBoundaryMessage struct {
	BaseMessage
	Type            MessageType     `json:"type"`
	Subtype         SystemSubtype   `json:"subtype"`
	CompactMetadata CompactMetadata `json:"compact_metadata"`
}

func (m CompactBoundaryMessage) GetType() MessageType { return MessageTypeSystem }

// CompactMetadata describes a compaction event.
type CompactMetadata struct {
	Trigger   string `json:"trigger"`
	PreTokens int    `json:"pre_tokens"`
}
