package events

import "github.com/koscakluka/ema-core/core/llms"

type CallToolEvent struct {
	BaseEvent
	Prompt string
	Tool   *llms.ToolCall
}

func (t CallToolEvent) String() string {
	if t.Tool == nil {
		return t.Prompt
	}

	return "Call tool " + t.Tool.Name + " with arguments " + t.Tool.Arguments
}

func NewCallToolWithPromptEvent(prompt string, opts ...RebaseOption) CallToolEvent {
	base := NewBaseEvent()
	for _, opt := range opts {
		opt(&base)
	}

	return CallToolEvent{
		BaseEvent: base,
		Prompt:    prompt,
	}
}

func NewCallToolEvent(tool llms.ToolCall, opts ...RebaseOption) CallToolEvent {
	base := NewBaseEvent()
	for _, opt := range opts {
		opt(&base)
	}

	return CallToolEvent{
		BaseEvent: base,
		Tool:      &tool,
	}
}
