package triggers

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
