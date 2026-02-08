package session

import (
	"bufio"
	"encoding/json"
	"os"

	"github.com/jg-phare/goat/pkg/agent"
)

const (
	messagesFile   = "messages.jsonl"
	transcriptFile = "transcript.jsonl"
	maxLineSize    = 10 * 1024 * 1024 // 10 MB
)

// appendJSONL marshals v as JSON and appends it as a single line to the file at path.
func appendJSONL(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

// loadMessageEntries reads all MessageEntry records from a JSONL file.
// Corrupt lines are skipped.
func loadMessageEntries(path string) ([]agent.MessageEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // empty session
		}
		return nil, err
	}
	defer f.Close()

	var entries []agent.MessageEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry agent.MessageEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip corrupt lines
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return entries, err
	}
	return entries, nil
}

// loadMessageEntriesUpTo reads MessageEntry records until the one with the given UUID (inclusive).
func loadMessageEntriesUpTo(path string, uuid string) ([]agent.MessageEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []agent.MessageEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry agent.MessageEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
		if entry.UUID == uuid {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return entries, err
	}
	return entries, nil
}
