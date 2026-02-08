package types

// HookStartedMessage is emitted when a hook begins execution.
type HookStartedMessage struct {
	BaseMessage
	Type      MessageType   `json:"type"`
	Subtype   SystemSubtype `json:"subtype"`
	HookID    string        `json:"hook_id"`
	HookName  string        `json:"hook_name"`
	HookEvent string        `json:"hook_event"`
}

func (m HookStartedMessage) GetType() MessageType { return MessageTypeSystem }

// HookProgressMessage streams hook stdout/stderr during execution.
type HookProgressMessage struct {
	BaseMessage
	Type      MessageType   `json:"type"`
	Subtype   SystemSubtype `json:"subtype"`
	HookID    string        `json:"hook_id"`
	HookName  string        `json:"hook_name"`
	HookEvent string        `json:"hook_event"`
	Stdout    string        `json:"stdout"`
	Stderr    string        `json:"stderr"`
	Output    string        `json:"output"`
}

func (m HookProgressMessage) GetType() MessageType { return MessageTypeSystem }

// HookResponseMessage is emitted when a hook completes.
type HookResponseMessage struct {
	BaseMessage
	Type      MessageType   `json:"type"`
	Subtype   SystemSubtype `json:"subtype"`
	HookID    string        `json:"hook_id"`
	HookName  string        `json:"hook_name"`
	HookEvent string        `json:"hook_event"`
	Output    string        `json:"output"`
	Stdout    string        `json:"stdout"`
	Stderr    string        `json:"stderr"`
	ExitCode  *int          `json:"exit_code,omitempty"`
	Outcome   string        `json:"outcome"`
}

func (m HookResponseMessage) GetType() MessageType { return MessageTypeSystem }
