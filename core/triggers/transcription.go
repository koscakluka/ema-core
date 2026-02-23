package triggers

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
