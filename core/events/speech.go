package events

type SpeechStartedEvent struct{ BaseEvent }

func (t SpeechStartedEvent) String() string { return "Speech Started" }

func NewSpeechStartedEvent(opts ...RebaseOption) SpeechStartedEvent {
	base := NewBaseEvent()
	for _, opt := range opts {
		opt(&base)
	}

	return SpeechStartedEvent{BaseEvent: base}
}

type SpeechEndedEvent struct{ BaseEvent }

func (t SpeechEndedEvent) String() string { return "Speech Ended" }

func NewSpeechEndedEvent(opts ...RebaseOption) SpeechEndedEvent {
	base := NewBaseEvent()
	for _, opt := range opts {
		opt(&base)
	}

	return SpeechEndedEvent{BaseEvent: base}
}
