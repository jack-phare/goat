package teams

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// TaskStatus represents the lifecycle state of a shared team task.
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "in_progress"
	TaskCompleted  TaskStatus = "completed"
)

// TeamTask represents a single task in the shared task list.
type TeamTask struct {
	ID          string     `json:"id"`
	Subject     string     `json:"subject"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	AssignedTo  string     `json:"assignedTo"`
	ClaimedBy   string     `json:"claimedBy"`
	DependsOn   []string   `json:"dependsOn"`
	CreatedBy   string     `json:"createdBy"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// SharedTaskList provides file-locked concurrent task management.
// Each task is stored as a separate JSON file in the directory.
type SharedTaskList struct {
	dir string
}

// NewSharedTaskList creates a SharedTaskList backed by the given directory.
// The directory is created if it does not exist.
func NewSharedTaskList(dir string) *SharedTaskList {
	return &SharedTaskList{dir: dir}
}

// Create adds a new task to the shared task list.
func (stl *SharedTaskList) Create(task TeamTask) error {
	if task.ID == "" {
		return fmt.Errorf("task ID is required")
	}

	taskPath := stl.taskPath(task.ID)
	if _, err := os.Stat(taskPath); err == nil {
		return fmt.Errorf("task %s already exists", task.ID)
	}

	if err := os.MkdirAll(stl.dir, 0o755); err != nil {
		return fmt.Errorf("create task directory: %w", err)
	}

	if task.Status == "" {
		task.Status = TaskPending
	}
	now := time.Now()
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = now
	}

	return stl.writeTask(task)
}

// Claim acquires a file lock and claims a pending task for the given agent.
func (stl *SharedTaskList) Claim(taskID, agentID string) error {
	lock := stl.lockForTask(taskID)
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("failed to acquire lock for task %s: %w", taskID, err)
	}
	defer lock.Unlock()

	task, err := stl.readTask(taskID)
	if err != nil {
		return err
	}

	if task.Status != TaskPending {
		return fmt.Errorf("task %s is not pending (status: %s)", taskID, task.Status)
	}
	if task.ClaimedBy != "" {
		return fmt.Errorf("task %s already claimed by %s", taskID, task.ClaimedBy)
	}

	// Check dependencies are all completed
	for _, depID := range task.DependsOn {
		dep, err := stl.readTask(depID)
		if err != nil {
			return fmt.Errorf("dependency %s: %w", depID, err)
		}
		if dep.Status != TaskCompleted {
			return fmt.Errorf("task %s blocked by incomplete dependency %s", taskID, depID)
		}
	}

	task.ClaimedBy = agentID
	task.Status = TaskInProgress
	task.UpdatedAt = time.Now()
	return stl.writeTask(task)
}

// Complete marks a task as completed.
func (stl *SharedTaskList) Complete(taskID string) error {
	lock := stl.lockForTask(taskID)
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("failed to acquire lock for task %s: %w", taskID, err)
	}
	defer lock.Unlock()

	task, err := stl.readTask(taskID)
	if err != nil {
		return err
	}

	if task.Status == TaskCompleted {
		return fmt.Errorf("task %s is already completed", taskID)
	}

	task.Status = TaskCompleted
	task.UpdatedAt = time.Now()
	return stl.writeTask(task)
}

// List returns all tasks in the shared task list, sorted by creation time.
func (stl *SharedTaskList) List() ([]TeamTask, error) {
	entries, err := os.ReadDir(stl.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read task directory: %w", err)
	}

	var tasks []TeamTask
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		taskID := strings.TrimSuffix(entry.Name(), ".json")
		task, err := stl.readTask(taskID)
		if err != nil {
			continue // skip unreadable tasks
		}
		tasks = append(tasks, task)
	}

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.Before(tasks[j].CreatedAt)
	})

	return tasks, nil
}

// GetUnblocked returns pending tasks whose dependencies are all completed.
func (stl *SharedTaskList) GetUnblocked() ([]TeamTask, error) {
	all, err := stl.List()
	if err != nil {
		return nil, err
	}

	// Build a status lookup
	statusMap := make(map[string]TaskStatus, len(all))
	for _, t := range all {
		statusMap[t.ID] = t.Status
	}

	var unblocked []TeamTask
	for _, t := range all {
		if t.Status != TaskPending {
			continue
		}
		if t.ClaimedBy != "" {
			continue
		}

		blocked := false
		for _, depID := range t.DependsOn {
			if statusMap[depID] != TaskCompleted {
				blocked = true
				break
			}
		}
		if !blocked {
			unblocked = append(unblocked, t)
		}
	}

	return unblocked, nil
}

// --- internal helpers ---

func (stl *SharedTaskList) taskPath(id string) string {
	return filepath.Join(stl.dir, id+".json")
}

func (stl *SharedTaskList) lockForTask(id string) *flock.Flock {
	return flock.New(filepath.Join(stl.dir, id+".lock"))
}

func (stl *SharedTaskList) readTask(id string) (TeamTask, error) {
	data, err := os.ReadFile(stl.taskPath(id))
	if err != nil {
		return TeamTask{}, fmt.Errorf("read task %s: %w", id, err)
	}

	var task TeamTask
	if err := json.Unmarshal(data, &task); err != nil {
		return TeamTask{}, fmt.Errorf("parse task %s: %w", id, err)
	}
	return task, nil
}

func (stl *SharedTaskList) writeTask(task TeamTask) error {
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal task %s: %w", task.ID, err)
	}
	return os.WriteFile(stl.taskPath(task.ID), data, 0o644)
}
