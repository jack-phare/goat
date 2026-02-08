package prompt

import (
	"strings"
	"testing"
)

func TestGetReminder_AllLoad(t *testing.T) {
	for _, id := range AllReminderIDs() {
		content := GetReminder(id, nil)
		if content == "" {
			t.Errorf("reminder %q loaded empty content", id)
		}
	}
}

func TestGetReminder_Count(t *testing.T) {
	ids := AllReminderIDs()
	if len(ids) < 37 {
		t.Errorf("expected at least 37 reminder IDs, got %d", len(ids))
	}
}

func TestGetReminder_UnknownID(t *testing.T) {
	content := GetReminder("nonexistent_reminder", nil)
	if content != "" {
		t.Errorf("expected empty string for unknown ID, got %q", content)
	}
}

func TestGetReminder_VariableSubstitution(t *testing.T) {
	// Token usage reminder uses ${ATTACHMENT_OBJECT.used}, ${ATTACHMENT_OBJECT.total}, etc.
	vars := map[string]string{
		"ATTACHMENT_OBJECT.used":      "50000",
		"ATTACHMENT_OBJECT.total":     "200000",
		"ATTACHMENT_OBJECT.remaining": "150000",
	}
	content := GetReminder(ReminderTokenUsage, vars)
	mustContain(t, content, "50000")
	mustContain(t, content, "200000")
	mustContain(t, content, "150000")
}

func TestGetReminder_FileTruncated(t *testing.T) {
	vars := map[string]string{
		"ATTACHMENT_OBJECT.filename": "main.go",
		"MAX_LINES_CONSTANT":        "2000",
		"READ_TOOL_OBJECT.name":     "Read",
	}
	content := GetReminder(ReminderFileTruncated, vars)
	mustContain(t, content, "main.go")
	mustContain(t, content, "2000")
	mustContain(t, content, "Read")
}

func TestGetReminder_FileModified(t *testing.T) {
	vars := map[string]string{
		"ATTACHMENT_OBJECT.filename": "app.tsx",
		"ATTACHMENT_OBJECT.snippet":  "+const x = 1;",
	}
	content := GetReminder(ReminderFileModified, vars)
	mustContain(t, content, "app.tsx")
	mustContain(t, content, "+const x = 1;")
}

func TestGetReminder_NoVars(t *testing.T) {
	// TodoWrite reminder has no variables to substitute (it's static text)
	content := GetReminder(ReminderTodoWriteReminder, nil)
	if content == "" {
		t.Error("expected non-empty content")
	}
	// Should contain the raw template text (unsubstituted)
	if !strings.Contains(content, "TodoWrite") {
		// It may use different naming
		mustContain(t, content, "todo")
	}
}
