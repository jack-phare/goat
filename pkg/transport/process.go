package transport

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
)

// ProcessAdapter wraps the Go agent runtime to look like a subprocess for
// SDK clients. It uses StdioTransport internally with io.Pipe pairs to
// provide Stdin/Stdout accessors.
type ProcessAdapter struct {
	// External endpoints (for the parent process)
	stdinWriter  io.WriteCloser // parent writes here → agent reads
	stdoutReader io.ReadCloser  // parent reads here ← agent writes

	// Internal pipes
	stdinReader  io.ReadCloser  // agent reads from this
	stdoutWriter io.WriteCloser // agent writes to this

	transport *StdioTransport
	router    *Router
	query     *agent.Query

	mu       sync.Mutex
	cancel   context.CancelFunc
	exitCode int32
	killed   atomic.Bool
	done     chan struct{}
	started  chan struct{} // closed when Run has set up cancel/transport
}

// NewProcessAdapter creates a ProcessAdapter that will use the given config
// to run an agent loop. Call Run() to start the loop.
func NewProcessAdapter() *ProcessAdapter {
	// Create pipe pairs
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	return &ProcessAdapter{
		stdinWriter:  stdinW,
		stdoutReader: stdoutR,
		stdinReader:  stdinR,
		stdoutWriter: stdoutW,
		done:         make(chan struct{}),
		started:      make(chan struct{}),
	}
}

// Stdin returns the writer for the parent process to send input to the agent.
// Write JSONL to this.
func (p *ProcessAdapter) Stdin() io.Writer {
	return p.stdinWriter
}

// Stdout returns the reader for the parent process to read output from the agent.
// Read JSONL from this.
func (p *ProcessAdapter) Stdout() io.Reader {
	return p.stdoutReader
}

// Run starts the agent loop with the given config and blocks until completion.
// The prompt is the initial user message.
func (p *ProcessAdapter) Run(ctx context.Context, prompt string, config agent.AgentConfig) error {
	ctx, cancel := context.WithCancel(ctx)

	// Ensure multi-turn is enabled for the adapter
	config.MultiTurn = true

	// Create the stdio transport over the internal pipes
	transport := NewStdioTransport(p.stdinReader, p.stdoutWriter)

	// Set fields under lock so Kill can safely access them
	p.mu.Lock()
	p.cancel = cancel
	p.transport = transport
	p.mu.Unlock()

	close(p.started) // signal that setup is complete

	// Start the agent loop
	p.query = agent.RunLoop(ctx, prompt, config)

	// Create and run the router
	p.router = NewRouter(transport, p.query)
	err := p.router.Run()

	// Clean up
	p.stdoutWriter.Close()
	p.stdinReader.Close()

	p.mu.Lock()
	if err != nil && !p.killed.Load() {
		atomic.StoreInt32(&p.exitCode, 1)
	}
	p.mu.Unlock()

	close(p.done)
	return err
}

// Kill terminates the agent loop by cancelling its context and closing pipes.
func (p *ProcessAdapter) Kill() {
	p.killed.Store(true)
	atomic.StoreInt32(&p.exitCode, -1)

	// Wait for Run to set up (with timeout to avoid blocking if Run was never called)
	select {
	case <-p.started:
	case <-time.After(5 * time.Second):
		// Run was never called or took too long; just close external endpoints
		p.stdinWriter.Close()
		return
	}

	p.mu.Lock()
	cancelFn := p.cancel
	transport := p.transport
	p.mu.Unlock()

	if cancelFn != nil {
		cancelFn()
	}
	if transport != nil {
		transport.Close()
	}
	// Close all pipe endpoints to unblock any waiting readers/writers
	p.stdinWriter.Close()
	p.stdinReader.Close()
	p.stdoutWriter.Close()
}

// Wait blocks until the adapter has finished running.
func (p *ProcessAdapter) Wait() {
	<-p.done
}

// ExitCode returns the exit code. 0 = success, 1 = error, -1 = killed.
func (p *ProcessAdapter) ExitCode() int {
	return int(atomic.LoadInt32(&p.exitCode))
}

// Killed returns true if Kill() was called.
func (p *ProcessAdapter) Killed() bool {
	return p.killed.Load()
}
