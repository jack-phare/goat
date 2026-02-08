package teams

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestTaskList(t *testing.T) *SharedTaskList {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tasks")
	return NewSharedTaskList(dir)
}

func TestTaskCreateAndList(t *testing.T) {
	stl := newTestTaskList(t)

	task := TeamTask{
		ID:        "task-1",
		Subject:   "Build feature",
		CreatedBy: "lead",
	}
	if err := stl.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	tasks, err := stl.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].ID != "task-1" {
		t.Errorf("expected task-1, got %s", tasks[0].ID)
	}
	if tasks[0].Status != TaskPending {
		t.Errorf("expected pending, got %s", tasks[0].Status)
	}
}

func TestTaskCreateDuplicate(t *testing.T) {
	stl := newTestTaskList(t)

	task := TeamTask{ID: "dup", Subject: "Test"}
	if err := stl.Create(task); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := stl.Create(task); err == nil {
		t.Fatal("expected error on duplicate create")
	}
}

func TestTaskCreateMissingID(t *testing.T) {
	stl := newTestTaskList(t)
	if err := stl.Create(TeamTask{Subject: "no ID"}); err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestTaskClaimAndComplete(t *testing.T) {
	stl := newTestTaskList(t)

	task := TeamTask{ID: "claim-1", Subject: "Work item", CreatedBy: "lead"}
	if err := stl.Create(task); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := stl.Claim("claim-1", "agent-a"); err != nil {
		t.Fatalf("Claim: %v", err)
	}

	// Verify status changed
	got, err := stl.readTask("claim-1")
	if err != nil {
		t.Fatalf("readTask: %v", err)
	}
	if got.Status != TaskInProgress {
		t.Errorf("expected in_progress, got %s", got.Status)
	}
	if got.ClaimedBy != "agent-a" {
		t.Errorf("expected agent-a, got %s", got.ClaimedBy)
	}

	// Complete the task
	if err := stl.Complete("claim-1"); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	got, _ = stl.readTask("claim-1")
	if got.Status != TaskCompleted {
		t.Errorf("expected completed, got %s", got.Status)
	}
}

func TestTaskClaimAlreadyClaimed(t *testing.T) {
	stl := newTestTaskList(t)

	task := TeamTask{ID: "claimed", Subject: "Work"}
	stl.Create(task)
	stl.Claim("claimed", "agent-a")

	err := stl.Claim("claimed", "agent-b")
	if err == nil {
		t.Fatal("expected error claiming already-claimed task")
	}
}

func TestTaskClaimCompletedFails(t *testing.T) {
	stl := newTestTaskList(t)

	task := TeamTask{ID: "done", Subject: "Done task"}
	stl.Create(task)
	stl.Claim("done", "agent-a")
	stl.Complete("done")

	err := stl.Claim("done", "agent-b")
	if err == nil {
		t.Fatal("expected error claiming completed task")
	}
}

func TestTaskCompleteAlreadyCompleted(t *testing.T) {
	stl := newTestTaskList(t)

	task := TeamTask{ID: "done2", Subject: "Already done"}
	stl.Create(task)
	stl.Claim("done2", "agent-a")
	stl.Complete("done2")

	if err := stl.Complete("done2"); err == nil {
		t.Fatal("expected error completing already-completed task")
	}
}

func TestTaskClaimNonexistent(t *testing.T) {
	stl := newTestTaskList(t)
	if err := stl.Claim("nonexistent", "agent-a"); err == nil {
		t.Fatal("expected error claiming nonexistent task")
	}
}

func TestTaskDependencyBlocking(t *testing.T) {
	stl := newTestTaskList(t)

	// Create dep task and dependent task
	dep := TeamTask{ID: "dep-a", Subject: "Prerequisite"}
	stl.Create(dep)

	dependent := TeamTask{ID: "dep-b", Subject: "Depends on dep-a", DependsOn: []string{"dep-a"}}
	stl.Create(dependent)

	// Can't claim dep-b while dep-a is pending
	err := stl.Claim("dep-b", "agent-x")
	if err == nil {
		t.Fatal("expected error claiming blocked task")
	}

	// Complete dep-a
	stl.Claim("dep-a", "agent-y")
	stl.Complete("dep-a")

	// Now dep-b should be claimable
	if err := stl.Claim("dep-b", "agent-x"); err != nil {
		t.Fatalf("expected dep-b to be claimable: %v", err)
	}
}

func TestTaskGetUnblocked(t *testing.T) {
	stl := newTestTaskList(t)

	// Create three tasks: a (no deps), b (depends on a), c (depends on a)
	stl.Create(TeamTask{ID: "u-a", Subject: "Task A"})
	stl.Create(TeamTask{ID: "u-b", Subject: "Task B", DependsOn: []string{"u-a"}})
	stl.Create(TeamTask{ID: "u-c", Subject: "Task C", DependsOn: []string{"u-a"}})

	// Only u-a is unblocked initially
	unblocked, err := stl.GetUnblocked()
	if err != nil {
		t.Fatalf("GetUnblocked: %v", err)
	}
	if len(unblocked) != 1 || unblocked[0].ID != "u-a" {
		t.Fatalf("expected [u-a], got %v", taskIDs(unblocked))
	}

	// Complete u-a
	stl.Claim("u-a", "agent-1")
	stl.Complete("u-a")

	// Now u-b and u-c should be unblocked
	unblocked, _ = stl.GetUnblocked()
	if len(unblocked) != 2 {
		t.Fatalf("expected 2 unblocked tasks, got %d: %v", len(unblocked), taskIDs(unblocked))
	}
}

func TestTaskGetUnblockedExcludesClaimed(t *testing.T) {
	stl := newTestTaskList(t)

	stl.Create(TeamTask{ID: "x-1", Subject: "Task 1"})
	stl.Create(TeamTask{ID: "x-2", Subject: "Task 2"})
	stl.Claim("x-1", "agent-a")

	unblocked, _ := stl.GetUnblocked()
	if len(unblocked) != 1 || unblocked[0].ID != "x-2" {
		t.Fatalf("expected [x-2], got %v", taskIDs(unblocked))
	}
}

func TestTaskConcurrentClaim(t *testing.T) {
	stl := newTestTaskList(t)

	task := TeamTask{ID: "race-task", Subject: "Race target"}
	stl.Create(task)

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines)

	successes := make(chan string, goroutines)

	for i := 0; i < goroutines; i++ {
		agentID := fmt.Sprintf("agent-%d", i)
		go func() {
			defer wg.Done()
			if err := stl.Claim("race-task", agentID); err == nil {
				successes <- agentID
			}
		}()
	}

	wg.Wait()
	close(successes)

	// Exactly one goroutine should have succeeded
	winners := 0
	for range successes {
		winners++
	}
	if winners != 1 {
		t.Fatalf("expected exactly 1 claim success, got %d", winners)
	}
}

func TestTaskListEmpty(t *testing.T) {
	stl := newTestTaskList(t)

	tasks, err := stl.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 0 {
		t.Fatalf("expected 0 tasks, got %d", len(tasks))
	}
}

func TestTaskListSortedByCreation(t *testing.T) {
	stl := newTestTaskList(t)

	now := time.Now()
	stl.Create(TeamTask{ID: "sort-b", Subject: "B", CreatedAt: now.Add(2 * time.Second)})
	stl.Create(TeamTask{ID: "sort-a", Subject: "A", CreatedAt: now})
	stl.Create(TeamTask{ID: "sort-c", Subject: "C", CreatedAt: now.Add(1 * time.Second)})

	tasks, _ := stl.List()
	if len(tasks) != 3 {
		t.Fatalf("expected 3, got %d", len(tasks))
	}
	if tasks[0].ID != "sort-a" || tasks[1].ID != "sort-c" || tasks[2].ID != "sort-b" {
		t.Errorf("unexpected order: %v", taskIDs(tasks))
	}
}

func TestTaskListIgnoresLockFiles(t *testing.T) {
	stl := newTestTaskList(t)

	stl.Create(TeamTask{ID: "lf-1", Subject: "Test"})

	// Create a .lock file manually
	os.WriteFile(filepath.Join(stl.dir, "lf-1.lock"), nil, 0o644)

	tasks, err := stl.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task (ignoring lock file), got %d", len(tasks))
	}
}

func taskIDs(tasks []TeamTask) []string {
	ids := make([]string, len(tasks))
	for i, t := range tasks {
		ids[i] = t.ID
	}
	return ids
}
