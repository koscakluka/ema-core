package context

import "github.com/koscakluka/ema-core/core/llms"

type TurnsV0 interface {
	Push(turn llms.Turn)
	Pop() *llms.Turn

	Clear()

	Values(yield func(llms.Turn) bool)
	RValues(yield func(llms.Turn) bool)
}

type ConversationV0 interface {
	Pop() *llms.TurnV1

	Clear()

	Values(yield func(llms.TurnV1) bool)
	RValues(yield func(llms.TurnV1) bool)
}
