package types

// Compile-time interface conformance checks.
// These verify all 16 message types implement SDKMessage.
var (
	_ SDKMessage = (*AssistantMessage)(nil)
	_ SDKMessage = (*PartialAssistantMessage)(nil)
	_ SDKMessage = (*UserMessage)(nil)
	_ SDKMessage = (*UserMessageReplay)(nil)
	_ SDKMessage = (*ResultMessage)(nil)
	_ SDKMessage = (*SystemInitMessage)(nil)
	_ SDKMessage = (*StatusMessage)(nil)
	_ SDKMessage = (*CompactBoundaryMessage)(nil)
	_ SDKMessage = (*HookStartedMessage)(nil)
	_ SDKMessage = (*HookProgressMessage)(nil)
	_ SDKMessage = (*HookResponseMessage)(nil)
	_ SDKMessage = (*ToolProgressMessage)(nil)
	_ SDKMessage = (*AuthStatusMessage)(nil)
	_ SDKMessage = (*TaskNotificationMessage)(nil)
	_ SDKMessage = (*FilesPersistedEvent)(nil)
	_ SDKMessage = (*ToolUseSummaryMessage)(nil)
)
