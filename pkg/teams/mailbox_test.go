package teams

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestMailbox(t *testing.T) *Mailbox {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "mailbox")
	return NewMailbox(dir)
}

func TestMailboxSendAndReceive(t *testing.T) {
	mb := newTestMailbox(t)

	msg := Message{
		From:    "lead",
		To:      "agent-1",
		Content: "Start working on task A",
		Type:    "message",
	}
	if err := mb.Send(msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	messages, err := mb.Receive("agent-1")
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].Content != "Start working on task A" {
		t.Errorf("unexpected content: %s", messages[0].Content)
	}
	if messages[0].From != "lead" {
		t.Errorf("unexpected from: %s", messages[0].From)
	}
}

func TestMailboxReceiveClearsMessages(t *testing.T) {
	mb := newTestMailbox(t)

	mb.Send(Message{From: "lead", To: "agent-1", Content: "Hello"})

	// First receive gets the message
	msgs, _ := mb.Receive("agent-1")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	// Second receive should be empty
	msgs, _ = mb.Receive("agent-1")
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages after clear, got %d", len(msgs))
	}
}

func TestMailboxReceiveEmpty(t *testing.T) {
	mb := newTestMailbox(t)

	msgs, err := mb.Receive("nobody")
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0, got %d", len(msgs))
	}
}

func TestMailboxBroadcast(t *testing.T) {
	mb := newTestMailbox(t)

	recipients := []string{"agent-1", "agent-2", "agent-3"}
	if err := mb.Broadcast("lead", "Team meeting at 3pm", recipients); err != nil {
		t.Fatalf("Broadcast: %v", err)
	}

	for _, r := range recipients {
		msgs, err := mb.Receive(r)
		if err != nil {
			t.Fatalf("Receive(%s): %v", r, err)
		}
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message for %s, got %d", r, len(msgs))
		}
		if msgs[0].Type != "broadcast" {
			t.Errorf("expected broadcast type, got %s", msgs[0].Type)
		}
		if msgs[0].Content != "Team meeting at 3pm" {
			t.Errorf("unexpected content for %s: %s", r, msgs[0].Content)
		}
	}
}

func TestMailboxSendMissingRecipient(t *testing.T) {
	mb := newTestMailbox(t)
	err := mb.Send(Message{From: "lead", Content: "oops"})
	if err == nil {
		t.Fatal("expected error for missing recipient")
	}
}

func TestMailboxSendDefaultFields(t *testing.T) {
	mb := newTestMailbox(t)

	msg := Message{From: "lead", To: "agent-1", Content: "test"}
	mb.Send(msg)

	msgs, _ := mb.Receive("agent-1")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].ID == "" {
		t.Error("expected auto-generated ID")
	}
	if msgs[0].Timestamp.IsZero() {
		t.Error("expected auto-generated timestamp")
	}
	if msgs[0].Type != "message" {
		t.Errorf("expected default type 'message', got %s", msgs[0].Type)
	}
}

func TestMailboxMultipleMessages(t *testing.T) {
	mb := newTestMailbox(t)

	for i := 0; i < 5; i++ {
		mb.Send(Message{
			From:      "lead",
			To:        "agent-1",
			Content:   fmt.Sprintf("Message %d", i),
			Timestamp: time.Now().Add(time.Duration(i) * time.Millisecond),
		})
	}

	msgs, _ := mb.Receive("agent-1")
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}

	// Verify sorted by timestamp
	for i := 1; i < len(msgs); i++ {
		if msgs[i].Timestamp.Before(msgs[i-1].Timestamp) {
			t.Errorf("messages not sorted by timestamp at index %d", i)
		}
	}
}

func TestMailboxConcurrentSend(t *testing.T) {
	mb := newTestMailbox(t)

	const senders = 10
	var wg sync.WaitGroup
	wg.Add(senders)

	for i := 0; i < senders; i++ {
		i := i
		go func() {
			defer wg.Done()
			mb.Send(Message{
				From:    fmt.Sprintf("sender-%d", i),
				To:      "agent-1",
				Content: fmt.Sprintf("Message from %d", i),
			})
		}()
	}

	wg.Wait()

	msgs, err := mb.Receive("agent-1")
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if len(msgs) != senders {
		t.Fatalf("expected %d messages, got %d", senders, len(msgs))
	}
}

func TestMailboxWatch(t *testing.T) {
	mb := newTestMailbox(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := mb.Watch(ctx, "watcher-agent")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	// Send a message
	mb.Send(Message{
		From:    "lead",
		To:      "watcher-agent",
		Content: "Hello via watch",
	})

	// Wait for the message on the channel
	select {
	case msg := <-ch:
		if msg.Content != "Hello via watch" {
			t.Errorf("unexpected content: %s", msg.Content)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for watched message")
	}
}

func TestMailboxWatchCancellation(t *testing.T) {
	mb := newTestMailbox(t)

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := mb.Watch(ctx, "cancel-agent")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	// Cancel and verify channel closes
	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			// Got a message, that's fine; channel should close next
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for channel close after cancel")
	}
}

func TestMailboxWatchMultipleMessages(t *testing.T) {
	mb := newTestMailbox(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := mb.Watch(ctx, "multi-watch")
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	// Send 3 messages with small delays to avoid filename collisions
	for i := 0; i < 3; i++ {
		mb.Send(Message{
			From:    "lead",
			To:      "multi-watch",
			Content: fmt.Sprintf("Msg %d", i),
		})
		time.Sleep(20 * time.Millisecond)
	}

	received := 0
	timeout := time.After(3 * time.Second)
	for received < 3 {
		select {
		case <-ch:
			received++
		case <-timeout:
			t.Fatalf("timed out after receiving %d/3 messages", received)
		}
	}
}
