package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"crypto/rand"
	"encoding/hex"
)

// TaskStatus represents the lifecycle state of a background task.
type TaskStatus int

const (
	TaskRunning   TaskStatus = iota
	TaskCompleted
	TaskFailed
	TaskStopped
)

func (s TaskStatus) String() string {
	switch s {
	case TaskRunning:
		return "running"
	case TaskCompleted:
		return "completed"
	case TaskFailed:
		return "failed"
	case TaskStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// taskOutput accumulates output from a background task in a thread-safe way.
type taskOutput struct {
	mu      sync.Mutex
	content strings.Builder
}

func (o *taskOutput) Write(s string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.content.WriteString(s)
}

func (o *taskOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.content.String()
}

// BackgroundTask represents a running or completed background task.
type BackgroundTask struct {
	ID        string
	Status    TaskStatus
	Output    *taskOutput
	Cancel    context.CancelFunc
	Done      chan struct{}
	StartedAt time.Time
	Error     error

	mu sync.Mutex // protects Status and Error
}

func (t *BackgroundTask) setStatus(s TaskStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Status = s
}

func (t *BackgroundTask) setError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Error = err
}

func (t *BackgroundTask) getStatus() TaskStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Status
}

func (t *BackgroundTask) getError() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.Error
}

// TaskManager tracks background tasks (bash commands, subagent loops).
type TaskManager struct {
	mu    sync.RWMutex
	tasks map[string]*BackgroundTask
}

// NewTaskManager creates a new TaskManager.
func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks: make(map[string]*BackgroundTask),
	}
}

// generateID returns a short random hex ID.
func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Launch starts a background task. The function fn runs in a goroutine and
// should return the task's output string (or error). The returned BackgroundTask
// can be used to track status.
func (tm *TaskManager) Launch(ctx context.Context, id string, fn func(ctx context.Context) (string, error)) *BackgroundTask {
	taskCtx, cancel := context.WithCancel(ctx)

	task := &BackgroundTask{
		ID:        id,
		Status:    TaskRunning,
		Output:    &taskOutput{},
		Cancel:    cancel,
		Done:      make(chan struct{}),
		StartedAt: time.Now(),
	}

	tm.mu.Lock()
	tm.tasks[id] = task
	tm.mu.Unlock()

	go func() {
		defer close(task.Done)
		result, err := fn(taskCtx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				task.setStatus(TaskStopped)
			} else {
				task.setStatus(TaskFailed)
				task.setError(err)
			}
			task.Output.Write(result)
		} else {
			task.Output.Write(result)
			task.setStatus(TaskCompleted)
		}
	}()

	return task
}

// Get retrieves a background task by ID.
func (tm *TaskManager) Get(id string) (*BackgroundTask, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	t, ok := tm.tasks[id]
	return t, ok
}

// GetOutput retrieves the output of a background task.
// If block is true, waits for the task to finish (up to timeout).
// If block is false, returns current partial output immediately.
func (tm *TaskManager) GetOutput(id string, block bool, timeout time.Duration) (string, error) {
	task, ok := tm.Get(id)
	if !ok {
		return "", fmt.Errorf("unknown task ID: %s", id)
	}

	if block {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-task.Done:
			// Task finished
		case <-timer.C:
			return task.Output.String(), fmt.Errorf("timeout waiting for task %s after %s", id, timeout)
		}
	}

	output := task.Output.String()
	status := task.getStatus()

	if status == TaskFailed {
		if err := task.getError(); err != nil {
			return output, fmt.Errorf("task %s failed: %w", id, err)
		}
	}

	return output, nil
}

// Stop cancels a running background task.
func (tm *TaskManager) Stop(id string) error {
	task, ok := tm.Get(id)
	if !ok {
		return fmt.Errorf("unknown task ID: %s", id)
	}

	status := task.getStatus()
	if status != TaskRunning {
		return fmt.Errorf("task %s is not running (status: %s)", id, status)
	}

	task.Cancel()
	// Wait briefly for the task to acknowledge cancellation
	select {
	case <-task.Done:
	case <-time.After(5 * time.Second):
	}

	return nil
}
