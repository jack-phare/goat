package teams

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
)

// TeamManager handles team lifecycle.
type TeamManager struct {
	mu         sync.RWMutex
	activeTeam *Team
	hooks      agent.HookRunner
	baseDir    string // root directory for teams data (e.g., ~/.claude)
}

// NewTeamManager creates a TeamManager.
func NewTeamManager(hooks agent.HookRunner, baseDir string) *TeamManager {
	return &TeamManager{
		hooks:   hooks,
		baseDir: baseDir,
	}
}

// CreateTeam creates a new team with the given name.
// Only one team per manager instance is allowed.
func (tm *TeamManager) CreateTeam(_ context.Context, name string) (*Team, error) {
	if name == "" {
		return nil, fmt.Errorf("team name is required")
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.activeTeam != nil {
		return nil, fmt.Errorf("a team is already active; clean up the current team first")
	}

	configPath := filepath.Join(tm.baseDir, "teams", name, "config.json")
	tasksDir := filepath.Join(tm.baseDir, "tasks", name)
	mailboxDir := filepath.Join(tm.baseDir, "teams", name, "mailbox")

	// Create directories
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return nil, fmt.Errorf("create team config directory: %w", err)
	}
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		return nil, fmt.Errorf("create tasks directory: %w", err)
	}
	if err := os.MkdirAll(mailboxDir, 0o755); err != nil {
		return nil, fmt.Errorf("create mailbox directory: %w", err)
	}

	team := &Team{
		Name:       name,
		Members:    make(map[string]*TeamMember),
		Tasks:      NewSharedTaskList(tasksDir),
		Mailbox:    NewMailbox(mailboxDir),
		Config:     TeamConfig{Name: name},
		CreatedAt:  time.Now(),
		configPath: configPath,
		tasksDir:   tasksDir,
		mailboxDir: mailboxDir,
	}

	if err := team.SaveConfig(); err != nil {
		return nil, fmt.Errorf("save initial config: %w", err)
	}

	tm.activeTeam = team
	return team, nil
}

// GetTeam returns the active team, or nil if no team is active.
func (tm *TeamManager) GetTeam() *Team {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.activeTeam
}

// Cleanup removes the active team and its resources.
// Fails if any teammates are still active.
func (tm *TeamManager) Cleanup(_ context.Context) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.activeTeam == nil {
		return fmt.Errorf("no active team")
	}

	if tm.activeTeam.HasActiveMembers() {
		return fmt.Errorf("team has active members; shut down all teammates before cleanup")
	}

	// Remove directories
	teamDir := filepath.Dir(tm.activeTeam.configPath)
	os.RemoveAll(teamDir)
	os.RemoveAll(tm.activeTeam.tasksDir)

	tm.activeTeam = nil
	return nil
}
