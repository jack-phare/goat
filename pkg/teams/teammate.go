package teams

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TeammateRuntime manages the lifecycle of a teammate process.
// It reads the team config, starts a mailbox watcher, and processes
// incoming messages as user turns in the agentic loop.
type TeammateRuntime struct {
	teamName  string
	agentName string
	agentType string
	baseDir   string

	mailbox *Mailbox
	tasks   *SharedTaskList
	config  TeamConfig
}

// NewTeammateRuntime creates a runtime for a teammate process.
func NewTeammateRuntime(teamName, agentName, agentType, baseDir string) *TeammateRuntime {
	mailboxDir := filepath.Join(baseDir, "teams", teamName, "mailbox")
	tasksDir := filepath.Join(baseDir, "tasks", teamName)

	return &TeammateRuntime{
		teamName:  teamName,
		agentName: agentName,
		agentType: agentType,
		baseDir:   baseDir,
		mailbox:   NewMailbox(mailboxDir),
		tasks:     NewSharedTaskList(tasksDir),
	}
}

// LoadConfig reads the team config from disk.
func (tr *TeammateRuntime) LoadConfig() error {
	configPath := filepath.Join(tr.baseDir, "teams", tr.teamName, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read team config: %w", err)
	}

	if err := json.Unmarshal(data, &tr.config); err != nil {
		return fmt.Errorf("parse team config: %w", err)
	}
	return nil
}

// WatchMessages starts watching for incoming messages.
// Returns a channel of messages and an error if the watcher can't start.
func (tr *TeammateRuntime) WatchMessages(ctx context.Context) (<-chan Message, error) {
	return tr.mailbox.Watch(ctx, tr.agentName)
}

// ReceiveMessages reads and removes all pending messages.
func (tr *TeammateRuntime) ReceiveMessages() ([]Message, error) {
	return tr.mailbox.Receive(tr.agentName)
}

// SendToLead sends a message to the team lead.
func (tr *TeammateRuntime) SendToLead(content, msgType string) error {
	return tr.mailbox.Send(Message{
		From:    tr.agentName,
		To:      "lead",
		Content: content,
		Type:    msgType,
	})
}

// NotifyIdle sends an idle notification to the team lead.
func (tr *TeammateRuntime) NotifyIdle() error {
	return tr.SendToLead(
		fmt.Sprintf("Teammate %s is idle and waiting for new work.", tr.agentName),
		"message",
	)
}

// RespondToShutdown sends a shutdown response.
func (tr *TeammateRuntime) RespondToShutdown(approve bool, reason string) error {
	content := "Shutdown approved."
	if !approve {
		content = fmt.Sprintf("Shutdown rejected: %s", reason)
	}
	return tr.mailbox.Send(Message{
		From:    tr.agentName,
		To:      "lead",
		Content: content,
		Type:    "shutdown_response",
	})
}

// ClaimTask claims an unblocked task from the shared task list.
func (tr *TeammateRuntime) ClaimTask(taskID string) error {
	return tr.tasks.Claim(taskID, tr.agentName)
}

// CompleteTask marks a task as completed.
func (tr *TeammateRuntime) CompleteTask(taskID string) error {
	return tr.tasks.Complete(taskID)
}

// GetUnblockedTasks returns tasks available for claiming.
func (tr *TeammateRuntime) GetUnblockedTasks() ([]TeamTask, error) {
	return tr.tasks.GetUnblocked()
}

// TeamName returns the team name.
func (tr *TeammateRuntime) TeamName() string { return tr.teamName }

// AgentName returns the agent name.
func (tr *TeammateRuntime) AgentName() string { return tr.agentName }

// AgentType returns the agent type.
func (tr *TeammateRuntime) AgentType() string { return tr.agentType }

// Config returns the loaded team config.
func (tr *TeammateRuntime) Config() TeamConfig { return tr.config }

// IsShutdownRequest checks if a message is a shutdown request.
func IsShutdownRequest(msg Message) bool {
	return msg.Type == "shutdown_request"
}
