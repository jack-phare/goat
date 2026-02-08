package types

// ToolProgressMessage is a heartbeat for long-running tool executions.
type ToolProgressMessage struct {
	BaseMessage
	Type               MessageType `json:"type"`
	ToolUseID          string      `json:"tool_use_id"`
	ToolName           string      `json:"tool_name"`
	ParentToolUseID    *string     `json:"parent_tool_use_id"`
	ElapsedTimeSeconds float64     `json:"elapsed_time_seconds"`
}

func (m ToolProgressMessage) GetType() MessageType { return MessageTypeToolProgress }

// AuthStatusMessage tracks OAuth/authentication flow status.
type AuthStatusMessage struct {
	BaseMessage
	Type             MessageType `json:"type"`
	IsAuthenticating bool        `json:"isAuthenticating"`
	Output           []string    `json:"output"`
	Error            string      `json:"error,omitempty"`
}

func (m AuthStatusMessage) GetType() MessageType { return MessageTypeAuthStatus }

// TaskNotificationMessage is emitted when a background subagent task completes.
type TaskNotificationMessage struct {
	BaseMessage
	Type       MessageType   `json:"type"`
	Subtype    SystemSubtype `json:"subtype"`
	TaskID     string        `json:"task_id"`
	Status     string        `json:"status"`
	OutputFile string        `json:"output_file"`
	Summary    string        `json:"summary"`
}

func (m TaskNotificationMessage) GetType() MessageType { return MessageTypeSystem }

// FilesPersistedEvent is emitted after session files are persisted to disk.
type FilesPersistedEvent struct {
	BaseMessage
	Type        MessageType     `json:"type"`
	Subtype     SystemSubtype   `json:"subtype"`
	Files       []PersistedFile `json:"files"`
	Failed      []FailedFile    `json:"failed"`
	ProcessedAt string          `json:"processed_at"`
}

func (m FilesPersistedEvent) GetType() MessageType { return MessageTypeSystem }

// PersistedFile describes a successfully persisted file.
type PersistedFile struct {
	Filename string `json:"filename"`
	FileID   string `json:"file_id"`
}

// FailedFile describes a file that failed to persist.
type FailedFile struct {
	Filename string `json:"filename"`
	Error    string `json:"error"`
}

// ToolUseSummaryMessage is injected during compaction to summarize tool use blocks.
type ToolUseSummaryMessage struct {
	BaseMessage
	Type                MessageType `json:"type"`
	Summary             string      `json:"summary"`
	PrecedingToolUseIDs []string    `json:"preceding_tool_use_ids"`
}

func (m ToolUseSummaryMessage) GetType() MessageType { return MessageTypeToolUseSummary }
