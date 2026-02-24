package events

const (
	// KindUserAudioFrame identifies raw audio captured from user input.
	KindUserAudioFrame Kind = "user_input.audio_frame"
	// KindUserSpeechStarted identifies start of user speech activity.
	KindUserSpeechStarted Kind = "user_input.speech_started"
	// KindUserSpeechEnded identifies end of user speech activity.
	KindUserSpeechEnded Kind = "user_input.speech_ended"
	// KindUserTranscriptInterimSegmentUpdated identifies mutable interim tail updates.
	KindUserTranscriptInterimSegmentUpdated Kind = "user_input.transcript_interim_segment_updated"
	// KindUserTranscriptInterimUpdated identifies mutable interim full transcript updates.
	KindUserTranscriptInterimUpdated Kind = "user_input.transcript_interim_updated"
	// KindUserTranscriptSegment identifies finalized append-only transcript segments.
	KindUserTranscriptSegment Kind = "user_input.transcript_segment"
	// KindUserTranscriptFinal identifies the final transcript for the utterance.
	KindUserTranscriptFinal Kind = "user_input.transcript_final"
)

// UserAudioFrame carries a user input audio frame.
type UserAudioFrame struct {
	Base
	Audio []byte
}

// NewUserAudioFrame creates a user input audio frame event.
func NewUserAudioFrame(audio []byte) UserAudioFrame {
	return UserAudioFrame{Base: NewBase(KindUserAudioFrame), Audio: audio}
}

// UserSpeechStarted marks when user speech activity starts.
type UserSpeechStarted struct{ Base }

// NewUserSpeechStarted creates a user speech started event.
func NewUserSpeechStarted() UserSpeechStarted {
	return UserSpeechStarted{Base: NewBase(KindUserSpeechStarted)}
}

// UserSpeechEnded marks when user speech activity ends.
type UserSpeechEnded struct{ Base }

// NewUserSpeechEnded creates a user speech ended event.
func NewUserSpeechEnded() UserSpeechEnded {
	return UserSpeechEnded{Base: NewBase(KindUserSpeechEnded)}
}

// UserTranscriptInterimSegmentUpdated carries a mutable interim transcript tail segment.
type UserTranscriptInterimSegmentUpdated struct {
	Base
	Segment string
}

// NewUserTranscriptInterimSegmentUpdated creates a mutable interim segment update event.
func NewUserTranscriptInterimSegmentUpdated(segment string) UserTranscriptInterimSegmentUpdated {
	return UserTranscriptInterimSegmentUpdated{Base: NewBase(KindUserTranscriptInterimSegmentUpdated), Segment: segment}
}

// UserTranscriptInterimUpdated carries the mutable interim full transcript snapshot.
type UserTranscriptInterimUpdated struct {
	Base
	Transcript string
}

// NewUserTranscriptInterimUpdated creates an interim transcript snapshot update event.
func NewUserTranscriptInterimUpdated(transcript string) UserTranscriptInterimUpdated {
	return UserTranscriptInterimUpdated{Base: NewBase(KindUserTranscriptInterimUpdated), Transcript: transcript}
}

// UserTranscriptSegment carries a finalized transcript segment.
type UserTranscriptSegment struct {
	Base
	Segment string
}

// NewUserTranscriptSegment creates a finalized transcript segment event.
func NewUserTranscriptSegment(segment string) UserTranscriptSegment {
	return UserTranscriptSegment{Base: NewBase(KindUserTranscriptSegment), Segment: segment}
}

// UserTranscriptFinal carries the final transcript for the utterance.
type UserTranscriptFinal struct {
	Base
	Transcript string
}

// NewUserTranscriptFinal creates a final transcript event.
func NewUserTranscriptFinal(transcript string) UserTranscriptFinal {
	return UserTranscriptFinal{Base: NewBase(KindUserTranscriptFinal), Transcript: transcript}
}
