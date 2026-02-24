package events

const (
	// KindAssistantPlaybackTranscriptUpdated identifies mutable playback transcript snapshots.
	KindAssistantPlaybackTranscriptUpdated Kind = "assistant_playback.transcript_updated"
	// KindAssistantPlaybackTranscriptSegment identifies append-only playback transcript segments.
	KindAssistantPlaybackTranscriptSegment Kind = "assistant_playback.transcript_segment"
	// KindAssistantPlaybackEnded identifies the playback completion milestone.
	KindAssistantPlaybackEnded Kind = "assistant_playback.ended"
)

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
