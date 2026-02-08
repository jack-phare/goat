package tools

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTaskManager_LaunchAndComplete(t *testing.T) {
	tm := NewTaskManager()
	task := tm.Launch(context.Background(), "t1", func(ctx context.Context) (string, error) {
		return "done", nil
	})

	<-task.Done

	if task.getStatus() != TaskCompleted {
		t.Fatalf("expected completed, got %s", task.getStatus())
	}
	output, err := tm.GetOutput("t1", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if output != "done" {
		t.Errorf("got %q, want %q", output, "done")
	}
}

func TestTaskManager_BlockingGetOutput(t *testing.T) {
	tm := NewTaskManager()
	tm.Launch(context.Background(), "t1", func(ctx context.Context) (string, error) {
		time.Sleep(50 * time.Millisecond)
		return "finished", nil
	})

	output, err := tm.GetOutput("t1", true, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if output != "finished" {
		t.Errorf("got %q, want %q", output, "finished")
	}
}

func TestTaskManager_NonBlockingPartial(t *testing.T) {
	tm := NewTaskManager()
	started := make(chan struct{})
	tm.Launch(context.Background(), "t1", func(ctx context.Context) (string, error) {
		close(started)
		// Block until cancelled
		<-ctx.Done()
		return "partial", ctx.Err()
	})

	<-started
	// Non-blocking should return immediately with empty output (task still running)
	output, err := tm.GetOutput("t1", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if output != "" {
		t.Errorf("expected empty partial output, got %q", output)
	}
}

func TestTaskManager_Stop(t *testing.T) {
	tm := NewTaskManager()
	started := make(chan struct{})
	tm.Launch(context.Background(), "t1", func(ctx context.Context) (string, error) {
		close(started)
		<-ctx.Done()
		return "", ctx.Err()
	})

	<-started
	if err := tm.Stop("t1"); err != nil {
		t.Fatal(err)
	}

	task, _ := tm.Get("t1")
	if task.getStatus() != TaskStopped {
		t.Errorf("expected stopped, got %s", task.getStatus())
	}
}

func TestTaskManager_TimeoutOnGetOutput(t *testing.T) {
	tm := NewTaskManager()
	tm.Launch(context.Background(), "t1", func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})

	_, err := tm.GetOutput("t1", true, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout error, got: %s", err)
	}
}

func TestTaskManager_UnknownID(t *testing.T) {
	tm := NewTaskManager()
	_, err := tm.GetOutput("nonexistent", false, 0)
	if err == nil {
		t.Fatal("expected error for unknown ID")
	}

	err = tm.Stop("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown ID")
	}
}

func TestTaskManager_FailedTask(t *testing.T) {
	tm := NewTaskManager()
	tm.Launch(context.Background(), "t1", func(ctx context.Context) (string, error) {
		return "partial output", errors.New("something broke")
	})

	output, err := tm.GetOutput("t1", true, 5*time.Second)
	if err == nil {
		t.Fatal("expected error for failed task")
	}
	if !strings.Contains(err.Error(), "something broke") {
		t.Errorf("expected original error, got: %s", err)
	}
	if output != "partial output" {
		t.Errorf("expected partial output, got %q", output)
	}
}

func TestTaskManager_ConcurrentAccess(t *testing.T) {
	tm := NewTaskManager()
	var wg sync.WaitGroup

	// Launch 10 tasks concurrently
	for i := range 10 {
		wg.Add(1)
		id := string(rune('a'+i)) + "task"
		go func() {
			defer wg.Done()
			tm.Launch(context.Background(), id, func(ctx context.Context) (string, error) {
				time.Sleep(10 * time.Millisecond)
				return "ok", nil
			})
		}()
	}
	wg.Wait()

	// Retrieve all outputs concurrently
	for i := range 10 {
		wg.Add(1)
		id := string(rune('a'+i)) + "task"
		go func() {
			defer wg.Done()
			output, err := tm.GetOutput(id, true, 5*time.Second)
			if err != nil {
				t.Errorf("task %s: %v", id, err)
			}
			if output != "ok" {
				t.Errorf("task %s: got %q, want %q", id, output, "ok")
			}
		}()
	}
	wg.Wait()
}

func TestTaskManager_StopAlreadyCompleted(t *testing.T) {
	tm := NewTaskManager()
	task := tm.Launch(context.Background(), "t1", func(ctx context.Context) (string, error) {
		return "done", nil
	})
	<-task.Done

	err := tm.Stop("t1")
	if err == nil {
		t.Fatal("expected error stopping completed task")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("expected 'not running' error, got: %s", err)
	}
}

func TestTaskStatus_String(t *testing.T) {
	tests := []struct {
		status TaskStatus
		want   string
	}{
		{TaskRunning, "running"},
		{TaskCompleted, "completed"},
		{TaskFailed, "failed"},
		{TaskStopped, "stopped"},
		{TaskStatus(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("TaskStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}
