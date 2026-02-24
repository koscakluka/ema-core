package events

const (
	// KindAssistantPlaybackStarted identifies playback start for the current response.
	KindAssistantPlaybackStarted Kind = "assistant_playback.started"
	// KindAssistantPlaybackFrame identifies an approximated playback audio delta frame.
	KindAssistantPlaybackFrame Kind = "assistant_playback.frame"
	// KindAssistantPlaybackMarkPlayed identifies confirmation that an output mark was played.
	KindAssistantPlaybackMarkPlayed Kind = "assistant_playback.mark_played"
	// KindAssistantPlaybackTranscriptUpdated identifies mutable playback transcript snapshots.
	KindAssistantPlaybackTranscriptUpdated Kind = "assistant_playback.transcript_updated"
	// KindAssistantPlaybackTranscriptSegment identifies append-only playback transcript segments.
	KindAssistantPlaybackTranscriptSegment Kind = "assistant_playback.transcript_segment"
	// KindAssistantPlaybackEnded identifies the playback completion milestone.
	KindAssistantPlaybackEnded Kind = "assistant_playback.ended"
)

// AssistantPlaybackStarted marks the start of assistant playback.
type AssistantPlaybackStarted struct{ Base }

// NewAssistantPlaybackStarted creates an assistant playback started event.
func NewAssistantPlaybackStarted() AssistantPlaybackStarted {
	return AssistantPlaybackStarted{Base: NewBase(KindAssistantPlaybackStarted)}
}

// AssistantPlaybackFrame carries an approximated append-only playback audio delta.
type AssistantPlaybackFrame struct {
	Base
	Audio []byte
}

// NewAssistantPlaybackFrame creates an assistant playback frame event.
func NewAssistantPlaybackFrame(audio []byte) AssistantPlaybackFrame {
	return AssistantPlaybackFrame{Base: NewBase(KindAssistantPlaybackFrame), Audio: audio}
}

// AssistantPlaybackMarkPlayed marks confirmation that a playback mark was played.
type AssistantPlaybackMarkPlayed struct {
	Base
	Mark       string
	Transcript string
}

// NewAssistantPlaybackMarkPlayed creates an assistant playback mark played event.
func NewAssistantPlaybackMarkPlayed(mark, transcript string) AssistantPlaybackMarkPlayed {
	return AssistantPlaybackMarkPlayed{Base: NewBase(KindAssistantPlaybackMarkPlayed), Mark: mark, Transcript: transcript}
}

// AssistantPlaybackTranscriptUpdated carries the current playback transcript snapshot.
type AssistantPlaybackTranscriptUpdated struct {
	Base
	Transcript string
}

// NewAssistantPlaybackTranscriptUpdated creates a playback transcript updated event.
func NewAssistantPlaybackTranscriptUpdated(transcript string) AssistantPlaybackTranscriptUpdated {
	return AssistantPlaybackTranscriptUpdated{Base: NewBase(KindAssistantPlaybackTranscriptUpdated), Transcript: transcript}
}

// AssistantPlaybackTranscriptSegment carries an append-only playback transcript segment.
type AssistantPlaybackTranscriptSegment struct {
	Base
	Segment string
}

// NewAssistantPlaybackTranscriptSegment creates a playback transcript segment event.
func NewAssistantPlaybackTranscriptSegment(segment string) AssistantPlaybackTranscriptSegment {
	return AssistantPlaybackTranscriptSegment{Base: NewBase(KindAssistantPlaybackTranscriptSegment), Segment: segment}
}

// AssistantPlaybackEnded marks the end of assistant playback.
type AssistantPlaybackEnded struct {
	Base
	Transcript string
}

// NewAssistantPlaybackEnded creates an assistant playback ended event.
func NewAssistantPlaybackEnded(transcript string) AssistantPlaybackEnded {
	return AssistantPlaybackEnded{Base: NewBase(KindAssistantPlaybackEnded), Transcript: transcript}
}
