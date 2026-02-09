package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jg-phare/goat/pkg/agent"
	"github.com/jg-phare/goat/pkg/llm"
	"github.com/jg-phare/goat/pkg/tools"
	"github.com/jg-phare/goat/pkg/types"
)

func processConfig(client llm.Client) agent.AgentConfig {
	return agent.AgentConfig{
		Model:          "claude-sonnet-4-5-20250929",
		MaxTurns:       100,
		PermissionMode: types.PermissionModeDefault,
		LLMClient:      client,
		ToolRegistry:   tools.NewRegistry(),
		Prompter:       &agent.StaticPromptAssembler{Prompt: "You are a test assistant."},
		Permissions:    &agent.AllowAllChecker{},
		Hooks:          &agent.NoOpHookRunner{},
		Compactor:      &agent.NoOpCompactor{},
		CostTracker:    llm.NewCostTracker(),
	}
}

func TestProcessAdapter_StdinStdoutRoundTrip(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStream{
			endTurnResponse("Hello from agent!"),
			endTurnResponse("Got your follow-up!"),
		},
	}
	config := processConfig(client)

	adapter := NewProcessAdapter()

	// Run the adapter in a goroutine
	runDone := make(chan error, 1)
	go func() {
		runDone <- adapter.Run(context.Background(), "Hello", config)
	}()

	// Read output from stdout (JSONL)
	outputLines := make(chan string, 64)
	go func() {
		scanner := bufio.NewScanner(adapter.Stdout())
		for scanner.Scan() {
			outputLines <- scanner.Text()
		}
		close(outputLines)
	}()

	// Wait for some output (the init message, assistant message, etc.)
	var outputs []string
	timeout := time.After(3 * time.Second)
	for {
		select {
		case line, ok := <-outputLines:
			if !ok {
				goto done
			}
			outputs = append(outputs, line)
			// After getting a result message (end of turn), send follow-up
			if strings.Contains(line, "result") {
				// Send a follow-up user message via stdin
				inputMsg := TransportMessage{
					Type:    TMsgOutput,
					Payload: json.RawMessage(`"Follow-up question"`),
				}
				data, _ := json.Marshal(inputMsg)
				data = append(data, '\n')
				adapter.Stdin().Write(data)

				// Wait briefly then kill
				time.Sleep(200 * time.Millisecond)
				adapter.Kill()
				goto done
			}
		case <-timeout:
			goto done
		}
	}

done:
	adapter.Wait()

	if len(outputs) == 0 {
		t.Error("expected at least 1 output line from stdout")
	}

	// Verify output is valid JSON
	for i, line := range outputs {
		if !json.Valid([]byte(line)) {
			t.Errorf("output[%d] is not valid JSON: %q", i, line)
		}
	}
}

func TestProcessAdapter_Kill(t *testing.T) {
	// Use a blocking client so we can test Kill during execution
	blockCh := make(chan struct{})
	client := &blockingClient{blockCh: blockCh}
	config := processConfig(client)

	adapter := NewProcessAdapter()

	runDone := make(chan error, 1)
	go func() {
		runDone <- adapter.Run(context.Background(), "Hello", config)
	}()

	// Wait briefly then kill
	time.Sleep(100 * time.Millisecond)
	adapter.Kill()

	// Unblock the client
	close(blockCh)

	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("adapter did not finish after Kill")
	}

	if !adapter.Killed() {
		t.Error("expected Killed() = true")
	}
	if adapter.ExitCode() != -1 {
		t.Errorf("ExitCode = %d, want -1", adapter.ExitCode())
	}
}

func TestProcessAdapter_GracefulShutdownOnStdinEOF(t *testing.T) {
	client := &mockLLMClient{
		responses: []*mockStream{
			endTurnResponse("Hello!"),
		},
	}
	config := processConfig(client)

	adapter := NewProcessAdapter()

	runDone := make(chan error, 1)
	go func() {
		runDone <- adapter.Run(context.Background(), "Hello", config)
	}()

	// Drain stdout
	go func() {
		scanner := bufio.NewScanner(adapter.Stdout())
		for scanner.Scan() {
		}
	}()

	// Wait for first turn, then close stdin
	time.Sleep(200 * time.Millisecond)
	adapter.Stdin().(interface{ Close() error }).Close()

	select {
	case <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("adapter did not finish after stdin close")
	}

	if adapter.ExitCode() != 0 {
		t.Errorf("ExitCode = %d, want 0", adapter.ExitCode())
	}
}

// blockingClient blocks until blockCh is closed.
type blockingClient struct {
	blockCh chan struct{}
}

func (b *blockingClient) Complete(ctx context.Context, req *llm.CompletionRequest) (*llm.Stream, error) {
	select {
	case <-b.blockCh:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return endTurnResponse("done").toStream(ctx), nil
}

func (b *blockingClient) Model() string    { return "test" }
func (b *blockingClient) SetModel(string) {}
