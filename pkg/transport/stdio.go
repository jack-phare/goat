package transport

import (
	"bufio"
	"encoding/json"
	"io"
	"sync"
	"sync/atomic"
)

const (
	// maxScannerBuffer is the max size for the JSONL scanner (10 MB).
	maxScannerBuffer = 10 * 1024 * 1024
	// initialScannerBuffer is the initial buffer size for the scanner (64 KB).
	initialScannerBuffer = 64 * 1024
)

// StdioTransport communicates via JSONL over stdin/stdout (or any io.Reader/Writer pair).
// Each line is a JSON-encoded TransportMessage. Empty lines are skipped.
type StdioTransport struct {
	reader io.Reader
	writer io.Writer

	inputCh  chan TransportMessage
	doneCh   chan struct{}
	ready    atomic.Bool
	writeMu  sync.Mutex
	closeOnce sync.Once
}

// NewStdioTransport creates a JSONL transport reading from reader and writing to writer.
// Call Close() when done to release resources.
func NewStdioTransport(reader io.Reader, writer io.Writer) *StdioTransport {
	t := &StdioTransport{
		reader:  reader,
		writer:  writer,
		inputCh: make(chan TransportMessage, 64),
		doneCh:  make(chan struct{}),
	}
	t.ready.Store(true)

	go t.readLoop()

	return t
}

// readLoop reads JSONL lines from the reader and sends TransportMessages on inputCh.
func (t *StdioTransport) readLoop() {
	defer close(t.inputCh)

	scanner := bufio.NewScanner(t.reader)
	scanner.Buffer(make([]byte, initialScannerBuffer), maxScannerBuffer)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue // skip empty lines
		}

		var msg TransportMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			// Malformed JSON → wrap as error message
			t.inputCh <- TransportMessage{
				Type:  TMsgError,
				Error: err,
			}
			continue
		}

		select {
		case t.inputCh <- msg:
		case <-t.doneCh:
			return
		}
	}

	// Scanner error (if any) is not fatal — just end of input
}

// Write sends JSON data as a single line to the writer.
// Thread-safe via mutex.
func (t *StdioTransport) Write(data []byte) error {
	if !t.ready.Load() {
		return ErrTransportClosed
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	// Write data + newline
	if _, err := t.writer.Write(data); err != nil {
		return err
	}
	if _, err := t.writer.Write([]byte{'\n'}); err != nil {
		return err
	}
	return nil
}

// Close shuts down the transport. Safe to call multiple times.
func (t *StdioTransport) Close() error {
	t.closeOnce.Do(func() {
		t.ready.Store(false)
		close(t.doneCh)
	})
	return nil
}

// IsReady returns true if the transport is accepting writes.
func (t *StdioTransport) IsReady() bool {
	return t.ready.Load()
}

// ReadMessages returns a channel of messages parsed from stdin.
// The channel is closed when stdin reaches EOF or the transport is closed.
func (t *StdioTransport) ReadMessages() <-chan TransportMessage {
	return t.inputCh
}

// EndInput is a no-op for stdio — stdin EOF is handled by scanner termination.
func (t *StdioTransport) EndInput() {}
