package triggers

import (
	"time"

	"github.com/koscakluka/ema-core/core/llms"
)

type UserPromptTrigger struct {
	BaseTrigger
	Prompt        string
	IsTranscribed bool
}

func (t UserPromptTrigger) String() string {
	return t.Prompt
}

func NewUserPromptTrigger(prompt string) UserPromptTrigger {
	return UserPromptTrigger{
		BaseTrigger:   NewBaseTrigger(),
		Prompt:        prompt,
		IsTranscribed: false,
	}
}

func NewTranscribedUserPromptTrigger(prompt string) UserPromptTrigger {
	return UserPromptTrigger{
		BaseTrigger:   NewBaseTrigger(),
		Prompt:        prompt,
		IsTranscribed: true,
	}
}

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

func NewCallToolWithPromptTrigger(prompt string) CallToolTrigger {
	return CallToolTrigger{
		BaseTrigger: NewBaseTrigger(),
		Prompt:      prompt,
	}
}

func NewCallToolTrigger(tool llms.ToolCall) CallToolTrigger {
	return CallToolTrigger{
		BaseTrigger: NewBaseTrigger(),
		Tool:        &tool,
	}
}

type BaseTrigger struct {
	timestamp time.Time
}

func NewBaseTrigger() BaseTrigger {
	return BaseTrigger{
		timestamp: time.Now(),
	}
}

func (t BaseTrigger) Timestamp() time.Time {
	return t.timestamp
}
