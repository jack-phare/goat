package subagent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManager_WatchReloadsOnFileChange(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	os.MkdirAll(agentDir, 0o755)

	mgr := newTestManager(&mockLLMClient{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start watching
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Watch(ctx, WatchDirs{ProjectDir: agentDir})
	}()

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Write a new agent file
	content := "---\ndescription: Hot-loaded agent\n---\nHot prompt."
	os.WriteFile(filepath.Join(agentDir, "hot-agent.md"), []byte(content), 0o644)

	// Wait for debounce + reload
	time.Sleep(500 * time.Millisecond)

	// Check the agent was loaded
	defs := mgr.Definitions()
	if _, ok := defs["hot-agent"]; !ok {
		t.Error("expected 'hot-agent' to be loaded after file creation")
	}

	cancel()
}

func TestManager_WatchReloadsOnDelete(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	os.MkdirAll(agentDir, 0o755)

	// Pre-create an agent file
	agentPath := filepath.Join(agentDir, "temp-agent.md")
	os.WriteFile(agentPath, []byte("---\ndescription: Temporary\n---\nPrompt."), 0o644)

	mgr := newTestManager(&mockLLMClient{})
	// Load it first
	mgr.Reload(dir)

	defs := mgr.Definitions()
	if _, ok := defs["temp-agent"]; !ok {
		t.Fatal("expected temp-agent before delete")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go mgr.Watch(ctx, WatchDirs{ProjectDir: agentDir})
	time.Sleep(100 * time.Millisecond)

	// Delete the file
	os.Remove(agentPath)

	// Wait for debounce + reload
	time.Sleep(500 * time.Millisecond)

	defs = mgr.Definitions()
	if _, ok := defs["temp-agent"]; ok {
		t.Error("expected temp-agent to be removed after file deletion")
	}

	cancel()
}

func TestManager_WatchDebounce(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	os.MkdirAll(agentDir, 0o755)

	mgr := newTestManager(&mockLLMClient{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go mgr.Watch(ctx, WatchDirs{ProjectDir: agentDir})
	time.Sleep(100 * time.Millisecond)

	// Rapid file changes (simulate rapid saves)
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(agentDir, "rapid.md"),
			[]byte("---\ndescription: Rapid write\n---\nPrompt."), 0o644)
		time.Sleep(20 * time.Millisecond)
	}

	// Wait for debounce
	time.Sleep(500 * time.Millisecond)

	// Should have the agent (single reload despite multiple writes)
	defs := mgr.Definitions()
	if _, ok := defs["rapid"]; !ok {
		t.Error("expected 'rapid' agent after debounced reload")
	}

	cancel()
}

func TestManager_WatchIgnoresNonMD(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	os.MkdirAll(agentDir, 0o755)

	mgr := newTestManager(&mockLLMClient{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go mgr.Watch(ctx, WatchDirs{ProjectDir: agentDir})
	time.Sleep(100 * time.Millisecond)

	// Write non-.md files — should not trigger reload
	os.WriteFile(filepath.Join(agentDir, "notes.txt"), []byte("not an agent"), 0o644)
	os.WriteFile(filepath.Join(agentDir, "config.yaml"), []byte("key: value"), 0o644)

	time.Sleep(500 * time.Millisecond)

	// No new agents should appear
	defs := mgr.Definitions()
	if _, ok := defs["notes"]; ok {
		t.Error("non-.md file should not create an agent definition")
	}

	cancel()
}

func TestManager_WatchCtxCancel(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, ".claude", "agents")
	os.MkdirAll(agentDir, 0o755)

	mgr := newTestManager(&mockLLMClient{})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Watch(ctx, WatchDirs{ProjectDir: agentDir})
	}()

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context
	cancel()

	// Watch should return
	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Watch did not return after context cancel")
	}
}
