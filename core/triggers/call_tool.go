package triggers

import "github.com/koscakluka/ema-core/core/llms"

type CallToolTrigger struct {
	BaseTrigger
	Prompt string
	Tool   *llms.ToolCall
}

func (t CallToolTrigger) String() string {
	if t.Tool == nil {
		return t.Prompt
	}

	return "Call tool " + t.Tool.Name + " with arguments " + t.Tool.Arguments
}

func NewCallToolWithPromptTrigger(prompt string, opts ...RebaseOption) CallToolTrigger {
	base := NewBaseTrigger()
	for _, opt := range opts {
		opt(&base)
	}

	return CallToolTrigger{
		BaseTrigger: base,
		Prompt:      prompt,
	}
}

func NewCallToolTrigger(tool llms.ToolCall, opts ...RebaseOption) CallToolTrigger {
	base := NewBaseTrigger()
	for _, opt := range opts {
		opt(&base)
	}

	return CallToolTrigger{
		BaseTrigger: base,
		Tool:        &tool,
	}
}
