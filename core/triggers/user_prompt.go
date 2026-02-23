package triggers

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
