package types

import (
	"encoding/json"
	"fmt"
)

// RawSDKMessage is used for first-pass deserialization to extract the discriminator.
type RawSDKMessage struct {
	Type    MessageType    `json:"type"`
	Subtype *SystemSubtype `json:"subtype,omitempty"`
}

// UnmarshalSDKMessage deserializes a JSON blob into the correct concrete SDKMessage type.
func UnmarshalSDKMessage(data []byte) (SDKMessage, error) {
	var raw RawSDKMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal discriminator: %w", err)
	}

	switch raw.Type {
	case MessageTypeAssistant:
		var msg AssistantMessage
		return &msg, json.Unmarshal(data, &msg)

	case MessageTypeUser:
		var probe struct {
			IsReplay bool `json:"isReplay"`
		}
		json.Unmarshal(data, &probe)
		if probe.IsReplay {
			var msg UserMessageReplay
			return &msg, json.Unmarshal(data, &msg)
		}
		var msg UserMessage
		return &msg, json.Unmarshal(data, &msg)

	case MessageTypeResult:
		var msg ResultMessage
		return &msg, json.Unmarshal(data, &msg)

	case MessageTypeSystem:
		return unmarshalSystemMessage(data, raw.Subtype)

	case MessageTypeStreamEvent:
		var msg PartialAssistantMessage
		return &msg, json.Unmarshal(data, &msg)

	case MessageTypeToolProgress:
		var msg ToolProgressMessage
		return &msg, json.Unmarshal(data, &msg)

	case MessageTypeAuthStatus:
		var msg AuthStatusMessage
		return &msg, json.Unmarshal(data, &msg)

	case MessageTypeToolUseSummary:
		var msg ToolUseSummaryMessage
		return &msg, json.Unmarshal(data, &msg)

	default:
		return nil, fmt.Errorf("unknown message type: %s", raw.Type)
	}
}

func unmarshalSystemMessage(data []byte, subtype *SystemSubtype) (SDKMessage, error) {
	if subtype == nil {
		return nil, fmt.Errorf("system message missing subtype")
	}
	switch *subtype {
	case SystemSubtypeInit:
		var msg SystemInitMessage
		return &msg, json.Unmarshal(data, &msg)
	case SystemSubtypeStatus:
		var msg StatusMessage
		return &msg, json.Unmarshal(data, &msg)
	case SystemSubtypeCompactBoundary:
		var msg CompactBoundaryMessage
		return &msg, json.Unmarshal(data, &msg)
	case SystemSubtypeHookStarted:
		var msg HookStartedMessage
		return &msg, json.Unmarshal(data, &msg)
	case SystemSubtypeHookProgress:
		var msg HookProgressMessage
		return &msg, json.Unmarshal(data, &msg)
	case SystemSubtypeHookResponse:
		var msg HookResponseMessage
		return &msg, json.Unmarshal(data, &msg)
	case SystemSubtypeFilesPersisted:
		var msg FilesPersistedEvent
		return &msg, json.Unmarshal(data, &msg)
	case SystemSubtypeTaskNotification:
		var msg TaskNotificationMessage
		return &msg, json.Unmarshal(data, &msg)
	default:
		return nil, fmt.Errorf("unknown system subtype: %s", *subtype)
	}
}
