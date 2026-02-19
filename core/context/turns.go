// Package context provides a modifiable interface for turns and conversations.
//
// Deprecated: use the [github.com/koscakluka/ema-core/core/conversations] package instead.
package context

import "github.com/koscakluka/ema-core/core/llms"

// TurnsV0 is a modifiable interface for turns.
//
// Deprecated: (since v0.0.17) use [github.com/koscakluka/ema-core/core/conversations.ActiveContextV0] instead.
type TurnsV0 interface {
	Push(turn llms.Turn)
	Pop() *llms.Turn

	Clear()

	Values(yield func(llms.Turn) bool)
	RValues(yield func(llms.Turn) bool)
}

// ConversationsV0 is a modifiable interface for conversations.
//
// Deprecated: (since v0.0.17) use [github.com/koscakluka/ema-core/core/conversations.ActiveContextV0] instead.
type ConversationV0 interface {
	Pop() *llms.TurnV1

	Clear()

	Values(yield func(llms.TurnV1) bool)
	RValues(yield func(llms.TurnV1) bool)
}
