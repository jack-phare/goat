package prompt

// ReminderID identifies a system reminder.
type ReminderID string

const (
	ReminderFileModified          ReminderID = "file_modified"
	ReminderFileTruncated         ReminderID = "file_truncated"
	ReminderFileEmpty             ReminderID = "file_empty"
	ReminderFileShorterThanOffset ReminderID = "file_shorter_than_offset"
	ReminderOutputTokenLimit      ReminderID = "output_token_limit"
	ReminderPlanModeActive5Phase  ReminderID = "plan_mode_active_5phase"
	ReminderPlanModeActiveIter    ReminderID = "plan_mode_active_iterative"
	ReminderPlanModeActiveSub     ReminderID = "plan_mode_active_subagent"
	ReminderPlanModeReEntry       ReminderID = "plan_mode_re_entry"
	ReminderExitedPlanMode        ReminderID = "exited_plan_mode"
	ReminderTodoWriteReminder     ReminderID = "todowrite_reminder"
	ReminderTodoListChanged       ReminderID = "todo_list_changed"
	ReminderTodoListEmpty         ReminderID = "todo_list_empty"
	ReminderTaskToolsReminder     ReminderID = "task_tools_reminder"
	ReminderTaskStatus            ReminderID = "task_status"
	ReminderTokenUsage            ReminderID = "token_usage"
	ReminderUSDBudget             ReminderID = "usd_budget"
	ReminderMalwareAnalysis       ReminderID = "malware_analysis"
	ReminderOutputStyleActive     ReminderID = "output_style_active"
	ReminderSessionContinuation   ReminderID = "session_continuation"
	ReminderVerifyPlan            ReminderID = "verify_plan"
	ReminderHookSuccess           ReminderID = "hook_success"
	ReminderHookBlockingError     ReminderID = "hook_blocking_error"
	ReminderHookStopped           ReminderID = "hook_stopped"
	ReminderHookContext           ReminderID = "hook_context"
	ReminderNewDiagnostics        ReminderID = "new_diagnostics"
	ReminderMemoryFile            ReminderID = "memory_file"
	ReminderNestedMemory          ReminderID = "nested_memory"
	ReminderCompactFileRef        ReminderID = "compact_file_ref"
	ReminderInvokedSkills         ReminderID = "invoked_skills"
	ReminderFileOpenedInIDE       ReminderID = "file_opened_in_ide"
	ReminderLinesSelectedInIDE    ReminderID = "lines_selected_in_ide"
	ReminderAgentMention          ReminderID = "agent_mention"
	ReminderBtwSideQuestion       ReminderID = "btw_side_question"
	ReminderMCPResourceNoContent  ReminderID = "mcp_resource_no_content"
	ReminderMCPResourceNoDisplay  ReminderID = "mcp_resource_no_displayable"
	ReminderPlanFileRef           ReminderID = "plan_file_ref"
)

// reminderFileMap maps reminder IDs to their embedded file names.
var reminderFileMap = map[ReminderID]string{
	ReminderFileModified:          "system-reminder-file-modified-by-user-or-linter.md",
	ReminderFileTruncated:         "system-reminder-file-truncated.md",
	ReminderFileEmpty:             "system-reminder-file-exists-but-empty.md",
	ReminderFileShorterThanOffset: "system-reminder-file-shorter-than-offset.md",
	ReminderOutputTokenLimit:      "system-reminder-output-token-limit-exceeded.md",
	ReminderPlanModeActive5Phase:  "system-reminder-plan-mode-is-active-5-phase.md",
	ReminderPlanModeActiveIter:    "system-reminder-plan-mode-is-active-iterative.md",
	ReminderPlanModeActiveSub:     "system-reminder-plan-mode-is-active-subagent.md",
	ReminderPlanModeReEntry:       "system-reminder-plan-mode-re-entry.md",
	ReminderExitedPlanMode:        "system-reminder-exited-plan-mode.md",
	ReminderTodoWriteReminder:     "system-reminder-todowrite-reminder.md",
	ReminderTodoListChanged:       "system-reminder-todo-list-changed.md",
	ReminderTodoListEmpty:         "system-reminder-todo-list-empty.md",
	ReminderTaskToolsReminder:     "system-reminder-task-tools-reminder.md",
	ReminderTaskStatus:            "system-reminder-task-status.md",
	ReminderTokenUsage:            "system-reminder-token-usage.md",
	ReminderUSDBudget:             "system-reminder-usd-budget.md",
	ReminderMalwareAnalysis:       "system-reminder-malware-analysis-after-read-tool-call.md",
	ReminderOutputStyleActive:     "system-reminder-output-style-active.md",
	ReminderSessionContinuation:   "system-reminder-session-continuation.md",
	ReminderVerifyPlan:            "system-reminder-verify-plan-reminder.md",
	ReminderHookSuccess:           "system-reminder-hook-success.md",
	ReminderHookBlockingError:     "system-reminder-hook-blocking-error.md",
	ReminderHookStopped:           "system-reminder-hook-stopped-continuation.md",
	ReminderHookContext:           "system-reminder-hook-additional-context.md",
	ReminderNewDiagnostics:        "system-reminder-new-diagnostics-detected.md",
	ReminderMemoryFile:            "system-reminder-memory-file-contents.md",
	ReminderNestedMemory:          "system-reminder-nested-memory-contents.md",
	ReminderCompactFileRef:        "system-reminder-compact-file-reference.md",
	ReminderInvokedSkills:         "system-reminder-invoked-skills.md",
	ReminderFileOpenedInIDE:       "system-reminder-file-opened-in-ide.md",
	ReminderLinesSelectedInIDE:    "system-reminder-lines-selected-in-ide.md",
	ReminderAgentMention:          "system-reminder-agent-mention.md",
	ReminderBtwSideQuestion:       "system-reminder-btw-side-question.md",
	ReminderMCPResourceNoContent:  "system-reminder-mcp-resource-no-content.md",
	ReminderMCPResourceNoDisplay:  "system-reminder-mcp-resource-no-displayable-content.md",
	ReminderPlanFileRef:           "system-reminder-plan-file-reference.md",
}

// GetReminder loads a system reminder by ID, performing variable substitution
// with the provided vars map. Returns "" if the reminder ID is unknown.
func GetReminder(id ReminderID, vars map[string]string) string {
	filename, ok := reminderFileMap[id]
	if !ok {
		return ""
	}
	content := loadReminderPrompt(filename)
	if content == "" {
		return ""
	}
	if len(vars) > 0 {
		content = simpleReplace(content, vars)
	}
	return content
}

// AllReminderIDs returns all known reminder IDs.
func AllReminderIDs() []ReminderID {
	ids := make([]ReminderID, 0, len(reminderFileMap))
	for id := range reminderFileMap {
		ids = append(ids, id)
	}
	return ids
}
