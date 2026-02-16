package events

type UserPromptEvent struct {
	BaseEvent
	Prompt        string
	IsTranscribed bool
}

func (t UserPromptEvent) String() string {
	return t.Prompt
}

func NewUserPromptEvent(prompt string, opts ...RebaseOption) UserPromptEvent {
	base := NewBaseEvent()
	for _, opt := range opts {
		opt(&base)
	}

	return UserPromptEvent{
		BaseEvent:     base,
		Prompt:        prompt,
		IsTranscribed: false,
	}
}

func NewTranscribedUserPromptEvent(prompt string, opts ...RebaseOption) UserPromptEvent {
	base := NewBaseEvent()
	for _, opt := range opts {
		opt(&base)
	}

	return UserPromptEvent{
		BaseEvent:     base,
		Prompt:        prompt,
		IsTranscribed: true,
	}
}
