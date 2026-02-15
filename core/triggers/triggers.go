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

func NewUserPromptTrigger(prompt string, opts ...RebaseOption) UserPromptTrigger {
	base := NewBaseTrigger()
	for _, opt := range opts {
		opt(&base)
	}

	return UserPromptTrigger{
		BaseTrigger:   base,
		Prompt:        prompt,
		IsTranscribed: false,
	}
}

func NewTranscribedUserPromptTrigger(prompt string, opts ...RebaseOption) UserPromptTrigger {
	base := NewBaseTrigger()
	for _, opt := range opts {
		opt(&base)
	}

	return UserPromptTrigger{
		BaseTrigger:   base,
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

type SpeechStartedTrigger struct{ BaseTrigger }

func (t SpeechStartedTrigger) String() string { return "Speech Started" }

func NewSpeechStartedTrigger(opts ...RebaseOption) SpeechStartedTrigger {
	base := NewBaseTrigger()
	for _, opt := range opts {
		opt(&base)
	}
	return SpeechStartedTrigger{BaseTrigger: base}
}

type SpeechEndedTrigger struct{ BaseTrigger }

func (t SpeechEndedTrigger) String() string { return "Speech Ended" }

func NewSpeechEndedTrigger(opts ...RebaseOption) SpeechEndedTrigger {
	base := NewBaseTrigger()
	for _, opt := range opts {
		opt(&base)
	}
	return SpeechEndedTrigger{BaseTrigger: base}
}

type InterimTranscriptionTrigger struct {
	BaseTrigger
	transcript string
}

func (t InterimTranscriptionTrigger) String() string     { return t.transcript + "..." }
func (t InterimTranscriptionTrigger) Transcript() string { return t.transcript }

func NewInterimTranscriptionTrigger(transcript string, opts ...RebaseOption) InterimTranscriptionTrigger {
	base := NewBaseTrigger()
	for _, opt := range opts {
		opt(&base)
	}
	return InterimTranscriptionTrigger{BaseTrigger: base, transcript: transcript}
}

type TranscriptionTrigger struct {
	BaseTrigger
	transcript string
}

func (t TranscriptionTrigger) String() string     { return t.transcript }
func (t TranscriptionTrigger) Transcript() string { return t.transcript }

func NewTranscriptionTrigger(transcript string, opts ...RebaseOption) TranscriptionTrigger {
	base := NewBaseTrigger()
	for _, opt := range opts {
		opt(&base)
	}
	return TranscriptionTrigger{BaseTrigger: base, transcript: transcript}
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

type RebaseOption func(*BaseTrigger)

func WithBase(base BaseTrigger) RebaseOption {
	return func(o *BaseTrigger) {
		*o = base
	}
}
