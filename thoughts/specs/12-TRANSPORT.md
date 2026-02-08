# Spec 12: Transport Layer

**Go Package**: `pkg/transport/`
**Source References**:
- `sdk.d.ts:1771-1795` — Transport interface (write, close, isReady, readMessages, endInput)
- `sdk.d.ts:1656-1710` — SpawnedProcess interface (stdin, stdout, killed, exitCode, kill, on/once/off)
- `sdk.d.ts:1712-1717` — SpawnOptions (command, args, cwd, env, signal)
- `sdk.d.ts:790-800` — Options.spawnClaudeCodeProcess (custom process spawner)
- `sdk.d.ts:1299-1320` — StdoutMessage types (control_response, output, error)

---

## 1. Purpose

The transport layer abstracts the communication channel between the agent runtime and its consumers (CLI, API, WebSocket). In the original Claude Code SDK, transport manages the stdin/stdout JSON-line protocol with the CLI subprocess. In our Go port, transport serves a different but analogous role: exposing the agent's message stream to external consumers.

**Key architectural difference**: The original SDK uses transport to talk to the CLI binary. Our Go port *is* the runtime, so transport wraps the agent loop's output for external consumers rather than subprocess communication.

---

## 2. Transport Interface (adapted from `sdk.d.ts:1771-1795`)

```go
// Transport abstracts the communication channel for the agent runtime.
type Transport interface {
    // Write sends a message to the agent (user input).
    Write(ctx context.Context, data []byte) error

    // Close terminates the transport and cleans up resources.
    Close() error

    // IsReady checks if the transport is accepting messages.
    IsReady() bool

    // ReadMessages returns a channel of messages from the agent.
    ReadMessages() <-chan TransportMessage

    // EndInput signals that no more input will be sent.
    EndInput()
}

// TransportMessage wraps an SDKMessage with transport metadata.
type TransportMessage struct {
    Type    TransportMessageType
    Payload json.RawMessage
    Error   error
}

type TransportMessageType string

const (
    TMsgOutput          TransportMessageType = "output"           // SDKMessage
    TMsgControlResponse TransportMessageType = "control_response" // response to control request
    TMsgError           TransportMessageType = "error"            // transport-level error
)
```

---

## 3. Transport Implementations

### 3.1 Channel Transport (in-process, for SDK-like usage)

```go
// ChannelTransport provides direct Go channel-based communication.
// Used when embedding the agent runtime as a library.
type ChannelTransport struct {
    input   chan []byte
    output  chan TransportMessage
    done    chan struct{}
    ready   atomic.Bool
}

func NewChannelTransport(bufferSize int) *ChannelTransport {
    t := &ChannelTransport{
        input:  make(chan []byte, bufferSize),
        output: make(chan TransportMessage, bufferSize),
        done:   make(chan struct{}),
    }
    t.ready.Store(true)
    return t
}

func (t *ChannelTransport) Write(_ context.Context, data []byte) error {
    if !t.ready.Load() {
        return ErrTransportClosed
    }
    t.input <- data
    return nil
}

func (t *ChannelTransport) ReadMessages() <-chan TransportMessage {
    return t.output
}

func (t *ChannelTransport) Close() error {
    t.ready.Store(false)
    close(t.done)
    return nil
}

func (t *ChannelTransport) IsReady() bool {
    return t.ready.Load()
}

func (t *ChannelTransport) EndInput() {
    close(t.input)
}
```

### 3.2 WebSocket Transport (for remote clients)

```go
// WebSocketTransport wraps a WebSocket connection for remote agent access.
// Used by the Phare Agent Hub API.
type WebSocketTransport struct {
    conn    *websocket.Conn
    output  chan TransportMessage
    done    chan struct{}
    mu      sync.Mutex
    ready   atomic.Bool
}

func NewWebSocketTransport(conn *websocket.Conn) *WebSocketTransport

func (t *WebSocketTransport) Write(ctx context.Context, data []byte) error {
    t.mu.Lock()
    defer t.mu.Unlock()
    return t.conn.WriteMessage(websocket.TextMessage, data)
}

func (t *WebSocketTransport) ReadMessages() <-chan TransportMessage {
    return t.output
}

// readPump runs in a goroutine, reading from the WebSocket and emitting messages.
func (t *WebSocketTransport) readPump() {
    defer close(t.output)
    for {
        _, message, err := t.conn.ReadMessage()
        if err != nil {
            if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
                t.output <- TransportMessage{Type: TMsgError, Error: err}
            }
            return
        }
        t.output <- TransportMessage{Type: TMsgOutput, Payload: message}
    }
}

func (t *WebSocketTransport) Close() error {
    t.ready.Store(false)
    close(t.done)
    return t.conn.Close()
}
```

### 3.3 HTTP SSE Transport (for API streaming)

```go
// SSETransport streams SDKMessages as Server-Sent Events over HTTP.
// Used by the Phare Agent Hub REST API for streaming responses.
type SSETransport struct {
    writer  http.ResponseWriter
    flusher http.Flusher
    output  chan TransportMessage
    done    chan struct{}
    ready   atomic.Bool
}

func NewSSETransport(w http.ResponseWriter) (*SSETransport, error) {
    flusher, ok := w.(http.Flusher)
    if !ok {
        return nil, fmt.Errorf("response writer does not support flushing")
    }

    // Set SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")

    t := &SSETransport{
        writer:  w,
        flusher: flusher,
        output:  make(chan TransportMessage, 64),
        done:    make(chan struct{}),
    }
    t.ready.Store(true)
    return t, nil
}

// EmitSSE writes an SSE event to the HTTP response.
func (t *SSETransport) EmitSSE(event string, data []byte) error {
    if !t.ready.Load() {
        return ErrTransportClosed
    }
    fmt.Fprintf(t.writer, "event: %s\ndata: %s\n\n", event, data)
    t.flusher.Flush()
    return nil
}
```

### 3.4 JSONL Stdio Transport (CLI compatibility)

```go
// StdioTransport communicates via stdin/stdout JSONL.
// Provides compatibility with the Claude Code SDK's process protocol.
type StdioTransport struct {
    stdin   io.Reader
    stdout  io.Writer
    scanner *bufio.Scanner
    output  chan TransportMessage
    done    chan struct{}
    mu      sync.Mutex
    ready   atomic.Bool
}

func NewStdioTransport(stdin io.Reader, stdout io.Writer) *StdioTransport {
    t := &StdioTransport{
        stdin:   stdin,
        stdout:  stdout,
        scanner: bufio.NewScanner(stdin),
        output:  make(chan TransportMessage, 64),
        done:    make(chan struct{}),
    }
    t.scanner.Buffer(make([]byte, 0), 10*1024*1024) // 10MB lines
    t.ready.Store(true)
    return t
}

func (t *StdioTransport) Write(_ context.Context, data []byte) error {
    t.mu.Lock()
    defer t.mu.Unlock()
    _, err := fmt.Fprintf(t.stdout, "%s\n", data)
    return err
}

// readPump reads JSONL from stdin.
func (t *StdioTransport) readPump() {
    defer close(t.output)
    for t.scanner.Scan() {
        line := t.scanner.Bytes()
        if len(line) == 0 { continue }
        t.output <- TransportMessage{
            Type:    TMsgOutput,
            Payload: json.RawMessage(append([]byte{}, line...)),
        }
    }
}
```

---

## 4. Control Protocol

The SDK uses control requests/responses for mid-session commands (setModel, interrupt, etc.):

```go
// ControlRequest is a command sent to the agent via transport.
type ControlRequest struct {
    Type    string          `json:"type"`    // "control_request"
    Command string          `json:"command"` // "interrupt", "setModel", "setPermissionMode", etc.
    Params  json.RawMessage `json:"params,omitempty"`
}

// ControlResponse is the agent's reply to a control request.
type ControlResponse struct {
    Type    string          `json:"type"`    // "control_response"
    Command string          `json:"command"`
    Success bool            `json:"success"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   string          `json:"error,omitempty"`
}

// Supported control commands (from Query interface, sdk.d.ts:948-1083):
const (
    CmdInterrupt           = "interrupt"
    CmdSetPermissionMode   = "setPermissionMode"
    CmdSetModel            = "setModel"
    CmdSetMaxThinkingTkns  = "setMaxThinkingTokens"
    CmdMCPServerStatus     = "mcpServerStatus"
    CmdReconnectMCP        = "reconnectMcpServer"
    CmdToggleMCP           = "toggleMcpServer"
    CmdSetMCPServers       = "setMcpServers"
    CmdRewindFiles         = "rewindFiles"
    CmdInitializationResult = "initializationResult"
    CmdSupportedCommands   = "supportedCommands"
    CmdSupportedModels     = "supportedModels"
    CmdAccountInfo         = "accountInfo"
)
```

---

## 5. Message Routing

```go
// Router connects a transport to an agent loop.
type Router struct {
    transport Transport
    agent     *agent.Query
    control   chan ControlRequest
}

func NewRouter(transport Transport, agent *agent.Query) *Router

// Run processes messages bidirectionally.
func (r *Router) Run(ctx context.Context) error {
    g, ctx := errgroup.WithContext(ctx)

    // Input: transport → agent
    g.Go(func() error {
        for msg := range r.transport.ReadMessages() {
            switch msg.Type {
            case TMsgOutput:
                // Parse as either user message or control request
                var cr ControlRequest
                if json.Unmarshal(msg.Payload, &cr) == nil && cr.Type == "control_request" {
                    r.handleControl(cr)
                } else {
                    r.agent.SendUserMessage(msg.Payload)
                }
            }
        }
        return nil
    })

    // Output: agent → transport
    g.Go(func() error {
        for msg := range r.agent.Messages {
            data, _ := json.Marshal(msg)
            r.transport.Write(ctx, data)
        }
        return nil
    })

    return g.Wait()
}
```

---

## 6. SpawnedProcess Compatibility (from `sdk.d.ts:1656-1710`)

For SDK compatibility (if wrapping our Go runtime as a subprocess), implement the SpawnedProcess interface:

```go
// ProcessAdapter wraps the Go agent runtime to look like a SpawnedProcess.
// Used when the Go binary is spawned by a Node.js SDK client.
type ProcessAdapter struct {
    stdin      io.ReadCloser
    stdout     io.WriteCloser
    transport  *StdioTransport
    agent      *agent.Query
    killed     atomic.Bool
    exitCode   atomic.Int32
    exitCh     chan struct{}
}

func NewProcessAdapter() *ProcessAdapter {
    stdinR, stdinW := io.Pipe()
    stdoutR, stdoutW := io.Pipe()
    return &ProcessAdapter{
        stdin:  stdinR,
        stdout: stdoutW,
        // Wire up transport to pipes
    }
}
```

---

## 7. Verification Checklist

- [ ] **Interface compliance**: All 4 transport implementations satisfy the Transport interface
- [ ] **Channel transport**: In-process communication works without serialization overhead
- [ ] **WebSocket transport**: Bidirectional message passing with proper close handling
- [ ] **SSE transport**: Correct event-stream format with flushing
- [ ] **Stdio transport**: JSONL protocol matches Claude Code SDK expectations
- [ ] **Control requests**: All 13 control commands from Query interface handled
- [ ] **Control responses**: Success/error responses routed back to requester
- [ ] **Backpressure**: Buffered channels prevent blocking on slow consumers
- [ ] **Graceful shutdown**: Close() cleanly terminates all goroutines
- [ ] **Large messages**: Handles messages up to 10MB (large tool outputs)
- [ ] **Concurrent writes**: Thread-safe write operations across all transports
- [ ] **Process adapter**: Go binary can be spawned by SDK clients via stdio
