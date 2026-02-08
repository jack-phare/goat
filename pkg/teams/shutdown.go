package teams

import (
	"context"
	"fmt"
	"os"
	"time"
)

// RequestShutdown sends a shutdown_request message to the teammate via the mailbox.
// The teammate is expected to respond by approving or rejecting the shutdown.
func (tm *TeamManager) RequestShutdown(_ context.Context, name string) error {
	tm.mu.RLock()
	team := tm.activeTeam
	tm.mu.RUnlock()

	if team == nil {
		return fmt.Errorf("no active team")
	}

	member, ok := team.GetMember(name)
	if !ok {
		return fmt.Errorf("unknown teammate: %s", name)
	}

	if member.GetState() == MemberStopped {
		return fmt.Errorf("teammate %s is already stopped", name)
	}

	return team.Mailbox.Send(Message{
		From:    "lead",
		To:      name,
		Content: "Shutdown requested by lead.",
		Type:    "shutdown_request",
	})
}

// ShutdownTeammate forcefully shuts down a teammate process.
// Sends SIGTERM first, waits up to timeout, then sends SIGKILL if needed.
func (tm *TeamManager) ShutdownTeammate(_ context.Context, name string, timeout time.Duration) error {
	tm.mu.RLock()
	team := tm.activeTeam
	tm.mu.RUnlock()

	if team == nil {
		return fmt.Errorf("no active team")
	}

	member, ok := team.GetMember(name)
	if !ok {
		return fmt.Errorf("unknown teammate: %s", name)
	}

	if member.GetState() == MemberStopped {
		return nil // already stopped
	}

	member.mu.Lock()
	proc := member.Process
	member.mu.Unlock()

	if proc != nil {
		// Send SIGTERM
		if err := proc.Signal(os.Interrupt); err != nil {
			// Process may already be dead
			member.SetState(MemberStopped)
			return nil
		}

		// Wait for graceful shutdown
		done := make(chan struct{})
		go func() {
			proc.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Graceful shutdown succeeded
		case <-time.After(timeout):
			// Force kill
			proc.Kill()
			<-done
		}
	}

	member.SetState(MemberStopped)

	// Notify lead via mailbox
	if team.Lead != nil {
		team.Mailbox.Send(Message{
			From:    name,
			To:      team.Lead.Name,
			Content: fmt.Sprintf("Teammate %s has shut down.", name),
			Type:    "message",
		})
	}

	return nil
}
