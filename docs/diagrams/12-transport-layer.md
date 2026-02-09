# Transport Layer — Pluggable I/O for Agent Communication

> `pkg/transport/` — Bidirectional bridge between the agent loop and external
> consumers. 4 transport implementations: Channel, Stdio, WebSocket, SSE.

## Transport Interface

```go
type Transport interface {
    Write(msg TransportMessage) error       // send message to consumer
    Close() error                            // shut down transport
    IsReady() bool                           // can accept messages?
    ReadMessages() <-chan TransportMessage    // receive from consumer
    EndInput()                               // signal no more input
}
```

## TransportMessage Envelope

```go
type TransportMessage struct {
    Type    MessageType     // "output" | "control_response" | "error"
    Payload json.RawMessage // the actual message data
    Error   string          // error description (for type="error")
}
```

## Router — Bidirectional Bridge

```
 ┌───────────────────────────────────────────────────────────────────┐
 │                          Router                                    │
 │                                                                   │
 │  Bridges Transport ↔ agent.Query                                  │
 │                                                                   │
 │  ┌─── Output Pump (goroutine) ──────────────────────────────┐   │
 │  │                                                           │   │
 │  │  for msg := range query.Messages():                       │   │
 │  │    transport.Write(TransportMessage{                      │   │
 │  │      Type: "output",                                      │   │
 │  │      Payload: json.Marshal(msg),                          │   │
 │  │    })                                                     │   │
 │  │                                                           │   │
 │  └───────────────────────────────────────────────────────────┘   │
 │                                                                   │
 │  ┌─── Input Pump (goroutine) ───────────────────────────────┐   │
 │  │                                                           │   │
 │  │  for msg := range transport.ReadMessages():               │   │
 │  │    switch msg.Type:                                       │   │
 │  │      "input":   query.SendUserMessage(msg.Payload)       │   │
 │  │      "control": query.SendControl(msg.Payload)           │   │
 │  │      "close":   query.Close()                            │   │
 │  │                                                           │   │
 │  └───────────────────────────────────────────────────────────┘   │
 │                                                                   │
 │  Lifecycle:                                                       │
 │    Start(query, transport) → launch both pumps                   │
 │    Wait() → block until query completes                          │
 │    Stop() → close transport, cancel context                      │
 └───────────────────────────────────────────────────────────────────┘
```

## 4 Transport Implementations

### ChannelTransport — In-Process Go Channels

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  Best for: Embedding Goat as a library in Go applications        │
 │                                                                   │
 │  inCh  chan TransportMessage (buffer: 64, configurable)          │
 │  outCh chan TransportMessage (buffer: 64, configurable)          │
 │                                                                   │
 │  Write(msg) → outCh <- msg                                       │
 │  ReadMessages() → inCh                                            │
 │                                                                   │
 │  Helpers:                                                         │
 │    Send(msg) → write to inCh (consumer sends to agent)           │
 │    Receive() → read from outCh (consumer reads from agent)       │
 │                                                                   │
 │  Zero serialization overhead — Go structs passed directly.       │
 │  Ideal for same-process integration.                              │
 └───────────────────────────────────────────────────────────────────┘
```

### StdioTransport — JSONL over stdin/stdout

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  Best for: CLI applications, piped processes                      │
 │                                                                   │
 │  reader  io.Reader (stdin or pipe)                                │
 │  writer  io.Writer (stdout or pipe)                               │
 │                                                                   │
 │  Write(msg):                                                      │
 │    json.Marshal(msg) + "\n" → writer                             │
 │    (one JSON object per line — JSONL format)                     │
 │                                                                   │
 │  ReadMessages():                                                  │
 │    readLoop goroutine:                                            │
 │      bufio.Scanner (10MB buffer) reads lines from reader         │
 │      json.Unmarshal → send to channel                            │
 │                                                                   │
 │  EndInput():                                                      │
 │    Close reader (EOF signals no more input)                      │
 │                                                                   │
 │  Compatible with subprocess IPC patterns.                        │
 └───────────────────────────────────────────────────────────────────┘
```

### WebSocketTransport — nhooyr.io/websocket

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  Best for: Web applications, real-time browser clients            │
 │                                                                   │
 │  conn  *websocket.Conn                                            │
 │                                                                   │
 │  Write(msg):                                                      │
 │    json.Marshal(msg)                                              │
 │    conn.Write(ctx, websocket.MessageText, data)                  │
 │    (text frames with JSON payload)                                │
 │                                                                   │
 │  ReadMessages():                                                  │
 │    readLoop goroutine:                                            │
 │      conn.Read(ctx) → json.Unmarshal → send to channel          │
 │                                                                   │
 │  Close():                                                         │
 │    conn.Close(websocket.StatusNormalClosure, "")                 │
 │                                                                   │
 │  Uses context-based read/write for cancellation.                 │
 │  nhooyr.io/websocket dependency (modern, context-aware).         │
 └───────────────────────────────────────────────────────────────────┘
```

### SSETransport — Server-Sent Events (Write-Only)

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  Best for: HTTP streaming to browser clients (one-way)            │
 │                                                                   │
 │  w  http.ResponseWriter (must implement http.Flusher)            │
 │                                                                   │
 │  Write(msg):                                                      │
 │    fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.Type, data)   │
 │    flusher.Flush()                                                │
 │                                                                   │
 │  ReadMessages():                                                  │
 │    Returns inputCh (populated externally via SendInput)          │
 │                                                                   │
 │  SendInput(msg):                                                  │
 │    inputCh <- msg  (caller provides input from separate path)   │
 │                                                                   │
 │  Write-only for the SSE stream. Input comes from a separate     │
 │  channel (e.g., POST endpoint alongside SSE).                    │
 │                                                                   │
 │  Validates http.Flusher at construction time.                    │
 └───────────────────────────────────────────────────────────────────┘
```

## ProcessAdapter — Agent as Subprocess

```
 ┌───────────────────────────────────────────────────────────────────┐
 │  ProcessAdapter                                                   │
 │                                                                   │
 │  Wraps an agent as a subprocess-like process:                    │
 │                                                                   │
 │  Stdin   io.WriteCloser  ←── external writes go here            │
 │  Stdout  io.ReadCloser   ←── agent output comes out here        │
 │                                                                   │
 │  Internally uses io.Pipe() pairs:                                │
 │    stdinReader ←────── stdinWriter (Stdin)                       │
 │    stdoutReader (Stdout) ←────── stdoutWriter                    │
 │                                                                   │
 │  Kill():                                                          │
 │    Cancel context → close all pipes                              │
 │                                                                   │
 │  ExitCode():                                                      │
 │    0 if completed normally, 1 if killed/errored                  │
 │                                                                   │
 │  Graceful shutdown:                                               │
 │    Close Stdin (EOF) → agent sees no more input → exits          │
 │                                                                   │
 │  Used by teams for teammate process simulation in tests.         │
 └───────────────────────────────────────────────────────────────────┘
```

## End-to-End Data Flow with Transport

```
 ┌─────────┐     ┌───────────┐     ┌──────────┐     ┌──────────────┐
 │ Consumer │────▶│ Transport │────▶│  Router  │────▶│  agent.Query │
 │ (UI/CLI/ │     │ (any of 4)│     │          │     │  (channels)  │
 │  web)    │     │           │     │          │     │              │
 │          │◀────│           │◀────│          │◀────│  RunLoop()   │
 │          │     │           │     │          │     │  goroutine   │
 └─────────┘     └───────────┘     └──────────┘     └──────────────┘

 Example with WebSocket:
   Browser connects via WS
   → WebSocketTransport wraps conn
   → Router bridges WS ↔ Query
   → User sends "Hello" via WS text frame
   → Router reads, calls query.SendUserMessage()
   → RunLoop processes, emits SDKMessages
   → Router reads messages, calls transport.Write()
   → WS sends text frame to browser
```

## Comparison with Claude Code TS

```
 ┌────────────────────────────┬──────────────────────────────────────┐
 │ Claude Code TS              │ Goat Go                              │
 ├────────────────────────────┼──────────────────────────────────────┤
 │ Terminal-only (Ink React)  │ Transport-agnostic (4 options)       │
 │ stdin/stdout hardcoded     │ Pluggable Transport interface        │
 │ No WebSocket support       │ WebSocket built-in                   │
 │ No SSE support             │ SSE built-in (write-only)            │
 │ No library embedding       │ ChannelTransport for in-process      │
 │ IPC via subprocess stdio   │ ProcessAdapter + StdioTransport      │
 └────────────────────────────┴──────────────────────────────────────┘

 This is where Goat significantly diverges from Claude Code TS.
 By abstracting transport, Goat can be embedded in:
   - CLI applications (StdioTransport)
   - Web servers (WebSocket/SSE)
   - Go applications (ChannelTransport)
   - Multi-process architectures (ProcessAdapter)
```
