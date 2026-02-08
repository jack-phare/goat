package teams

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
)

// Message represents an inter-agent message.
type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Content   string    `json:"content"`
	Summary   string    `json:"summary,omitempty"`
	Type      string    `json:"type"` // message, broadcast, shutdown_request, etc.
	Timestamp time.Time `json:"timestamp"`
}

// Mailbox handles inter-agent messaging via the filesystem.
// Each agent has its own inbox directory; messages are JSON files.
type Mailbox struct {
	dir string // base mailbox directory
}

// NewMailbox creates a Mailbox rooted at the given directory.
func NewMailbox(dir string) *Mailbox {
	return &Mailbox{dir: dir}
}

// Send writes a message to the recipient's inbox directory.
func (mb *Mailbox) Send(msg Message) error {
	if msg.To == "" {
		return fmt.Errorf("message recipient (To) is required")
	}
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	if msg.Type == "" {
		msg.Type = "message"
	}

	inboxDir := mb.inboxDir(msg.To)
	if err := os.MkdirAll(inboxDir, 0o755); err != nil {
		return fmt.Errorf("create inbox for %s: %w", msg.To, err)
	}

	filename := fmt.Sprintf("%d-%s.json", msg.Timestamp.UnixNano(), msg.ID)
	path := filepath.Join(inboxDir, filename)

	data, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// Broadcast sends a message to multiple recipients.
func (mb *Mailbox) Broadcast(from, content string, recipients []string) error {
	for _, to := range recipients {
		msg := Message{
			From:    from,
			To:      to,
			Content: content,
			Type:    "broadcast",
		}
		if err := mb.Send(msg); err != nil {
			return fmt.Errorf("broadcast to %s: %w", to, err)
		}
	}
	return nil
}

// Receive reads and removes all messages from an agent's inbox.
// Messages are returned sorted by timestamp (oldest first).
func (mb *Mailbox) Receive(agentName string) ([]Message, error) {
	inboxDir := mb.inboxDir(agentName)
	entries, err := os.ReadDir(inboxDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read inbox for %s: %w", agentName, err)
	}

	var messages []Message
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(inboxDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		// Remove after reading
		os.Remove(path)
		messages = append(messages, msg)
	}

	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp.Before(messages[j].Timestamp)
	})

	return messages, nil
}

// Watch monitors an agent's inbox for new messages using fsnotify.
// New messages are sent on the returned channel. The watcher stops
// when the context is cancelled.
func (mb *Mailbox) Watch(ctx context.Context, agentName string) (<-chan Message, error) {
	inboxDir := mb.inboxDir(agentName)
	if err := os.MkdirAll(inboxDir, 0o755); err != nil {
		return nil, fmt.Errorf("create inbox for %s: %w", agentName, err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}

	if err := watcher.Add(inboxDir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("watch inbox: %w", err)
	}

	ch := make(chan Message, 16)

	go func() {
		defer watcher.Close()
		defer close(ch)

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Create == 0 {
					continue
				}
				if !strings.HasSuffix(event.Name, ".json") {
					continue
				}

				// Small delay to let the file finish writing
				time.Sleep(5 * time.Millisecond)

				data, err := os.ReadFile(event.Name)
				if err != nil {
					continue
				}

				var msg Message
				if err := json.Unmarshal(data, &msg); err != nil {
					continue
				}

				// Remove after reading
				os.Remove(event.Name)

				select {
				case ch <- msg:
				case <-ctx.Done():
					return
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()

	return ch, nil
}

func (mb *Mailbox) inboxDir(agentName string) string {
	return filepath.Join(mb.dir, agentName)
}
