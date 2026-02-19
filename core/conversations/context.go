package conversations

import "github.com/koscakluka/ema-core/core/llms"

// ActiveContextV0 exposes live conversation context for event handlers.
type ActiveContextV0 interface {
	// Past turns only. Ordering: oldest -> newest.
	History() []llms.TurnV1

	// Current in-flight turn; nil when absent.
	ActiveTurn() *llms.TurnV1

	// Tools available in this conversation.
	AvailableTools() []llms.Tool
}
