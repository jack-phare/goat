package teams

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MemberState represents the lifecycle state of a team member.
type MemberState int

const (
	MemberActive  MemberState = iota
	MemberIdle
	MemberStopped
)

// String returns a human-readable label for the state.
func (s MemberState) String() string {
	switch s {
	case MemberActive:
		return "active"
	case MemberIdle:
		return "idle"
	case MemberStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// TeamConfig is the JSON-serialized team configuration.
type TeamConfig struct {
	Name    string         `json:"name"`
	Members []MemberConfig `json:"members"`
}

// MemberConfig is the JSON-serialized member entry in the team config.
type MemberConfig struct {
	Name      string `json:"name"`
	AgentID   string `json:"agentId"`
	AgentType string `json:"agentType"`
	PID       int    `json:"pid,omitempty"`
}

// TeamMember represents a teammate process.
type TeamMember struct {
	Name      string
	AgentID   string
	AgentType string
	State     MemberState
	PID       int
	Process   *os.Process

	mu sync.Mutex
}

// SetState updates the member's state in a thread-safe manner.
func (m *TeamMember) SetState(state MemberState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.State = state
}

// GetState returns the member's current state.
func (m *TeamMember) GetState() MemberState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.State
}

// IsAlive checks if the member's process is still running.
func (m *TeamMember) IsAlive() bool {
	m.mu.Lock()
	p := m.Process
	m.mu.Unlock()

	if p == nil {
		return false
	}

	// On Unix, Signal(0) checks process existence without sending a signal
	err := p.Signal(os.Signal(nil))
	return err == nil
}

// Team represents an active agent team.
type Team struct {
	mu         sync.RWMutex
	Name       string
	Lead       *TeamMember
	Members    map[string]*TeamMember // keyed by name
	Tasks      *SharedTaskList
	Mailbox    *Mailbox
	Config     TeamConfig
	CreatedAt  time.Time
	configPath string
	tasksDir   string
	mailboxDir string
}

// AddMember adds a member to the team and persists the config.
func (t *Team) AddMember(member *TeamMember) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Members[member.Name] = member
	return t.saveConfigLocked()
}

// GetMember returns a member by name.
func (t *Team) GetMember(name string) (*TeamMember, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	m, ok := t.Members[name]
	return m, ok
}

// MemberNames returns all member names.
func (t *Team) MemberNames() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	names := make([]string, 0, len(t.Members))
	for name := range t.Members {
		names = append(names, name)
	}
	return names
}

// HasActiveMembers returns true if any member is in the Active state.
func (t *Team) HasActiveMembers() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, m := range t.Members {
		if m.GetState() == MemberActive {
			return true
		}
	}
	return false
}

// saveConfigLocked writes the team config to disk. Caller must hold t.mu.
func (t *Team) saveConfigLocked() error {
	t.Config.Members = make([]MemberConfig, 0, len(t.Members))
	for _, m := range t.Members {
		t.Config.Members = append(t.Config.Members, MemberConfig{
			Name:      m.Name,
			AgentID:   m.AgentID,
			AgentType: m.AgentType,
			PID:       m.PID,
		})
	}

	data, err := json.MarshalIndent(t.Config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal team config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(t.configPath), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	return os.WriteFile(t.configPath, data, 0o644)
}

// SaveConfig persists the team config to disk (public, acquires lock).
func (t *Team) SaveConfig() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.saveConfigLocked()
}
