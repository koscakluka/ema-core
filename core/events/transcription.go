package events

type InterimTranscriptionEvent struct {
	BaseEvent
	transcript string
}

func (t InterimTranscriptionEvent) String() string     { return t.transcript + "..." }
func (t InterimTranscriptionEvent) Transcript() string { return t.transcript }

func NewInterimTranscriptionEvent(transcript string, opts ...RebaseOption) InterimTranscriptionEvent {
	base := NewBaseEvent()
	for _, opt := range opts {
		opt(&base)
	}

	return InterimTranscriptionEvent{BaseEvent: base, transcript: transcript}
}

type TranscriptionEvent struct {
	BaseEvent
	transcript string
}

func (t TranscriptionEvent) String() string     { return t.transcript }
func (t TranscriptionEvent) Transcript() string { return t.transcript }

func NewTranscriptionEvent(transcript string, opts ...RebaseOption) TranscriptionEvent {
	base := NewBaseEvent()
	for _, opt := range opts {
		opt(&base)
	}

	return TranscriptionEvent{BaseEvent: base, transcript: transcript}
}
