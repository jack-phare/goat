package teams

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/google/uuid"
)

// SpawnTeammate creates a new teammate process via exec.Command self-invocation.
func (tm *TeamManager) SpawnTeammate(ctx context.Context, name, agentType, prompt string) (*TeamMember, error) {
	if name == "" {
		return nil, fmt.Errorf("teammate name is required")
	}
	if agentType == "" {
		return nil, fmt.Errorf("agent type is required")
	}

	tm.mu.RLock()
	team := tm.activeTeam
	tm.mu.RUnlock()

	if team == nil {
		return nil, fmt.Errorf("no active team; create a team first")
	}

	if _, exists := team.GetMember(name); exists {
		return nil, fmt.Errorf("teammate %s already exists", name)
	}

	agentID := uuid.New().String()

	// Build self-invocation command
	cmd := exec.CommandContext(ctx, os.Args[0],
		"--teammate",
		"--team-name", team.Name,
		"--agent-name", name,
		"--agent-type", agentType,
	)

	// Set environment
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("CLAUDE_CODE_TEAM=%s", team.Name),
		fmt.Sprintf("CLAUDE_CODE_AGENT_NAME=%s", name),
		fmt.Sprintf("CLAUDE_CODE_AGENT_ID=%s", agentID),
		fmt.Sprintf("CLAUDE_CODE_BASE_DIR=%s", tm.baseDir),
	)

	// Start process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("spawn teammate %s: %w", name, err)
	}

	member := &TeamMember{
		Name:      name,
		AgentID:   agentID,
		AgentType: agentType,
		State:     MemberActive,
		PID:       cmd.Process.Pid,
		Process:   cmd.Process,
	}

	if err := team.AddMember(member); err != nil {
		// Best-effort kill if config update fails
		cmd.Process.Kill()
		return nil, fmt.Errorf("update team config: %w", err)
	}

	// Send initial prompt to teammate's mailbox
	if prompt != "" {
		team.Mailbox.Send(Message{
			From:    "lead",
			To:      name,
			Content: prompt,
			Type:    "message",
		})
	}

	// Wait for process in background and update state when done
	go func() {
		cmd.Wait()
		member.SetState(MemberStopped)
	}()

	return member, nil
}

// SpawnFunc is the function signature for spawning a teammate process.
// This is used for testing to avoid actual exec.Command calls.
type SpawnFunc func(ctx context.Context, name, teamName, agentType, baseDir, agentID string) (*os.Process, error)

// SpawnTeammateWithFunc creates a teammate using a custom spawn function.
// This is the testable variant of SpawnTeammate.
func (tm *TeamManager) SpawnTeammateWithFunc(ctx context.Context, name, agentType, prompt string, spawnFn SpawnFunc) (*TeamMember, error) {
	if name == "" {
		return nil, fmt.Errorf("teammate name is required")
	}
	if agentType == "" {
		return nil, fmt.Errorf("agent type is required")
	}

	tm.mu.RLock()
	team := tm.activeTeam
	tm.mu.RUnlock()

	if team == nil {
		return nil, fmt.Errorf("no active team; create a team first")
	}

	if _, exists := team.GetMember(name); exists {
		return nil, fmt.Errorf("teammate %s already exists", name)
	}

	agentID := uuid.New().String()

	proc, err := spawnFn(ctx, name, team.Name, agentType, tm.baseDir, agentID)
	if err != nil {
		return nil, fmt.Errorf("spawn teammate %s: %w", name, err)
	}

	pid := 0
	if proc != nil {
		pid = proc.Pid
	}

	member := &TeamMember{
		Name:      name,
		AgentID:   agentID,
		AgentType: agentType,
		State:     MemberActive,
		PID:       pid,
		Process:   proc,
	}

	if err := team.AddMember(member); err != nil {
		if proc != nil {
			proc.Kill()
		}
		return nil, fmt.Errorf("update team config: %w", err)
	}

	// Send initial prompt
	if prompt != "" {
		team.Mailbox.Send(Message{
			From:    "lead",
			To:      name,
			Content: prompt,
			Type:    "message",
		})
	}

	return member, nil
}
