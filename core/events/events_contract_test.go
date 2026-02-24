package events

import "testing"

func TestConstructorsEmitExpectedKinds(t *testing.T) {
	testCases := []struct {
		name     string
		event    Event
		expected Kind
	}{
		{name: "user audio frame", event: NewUserAudioFrame([]byte{1}), expected: KindUserAudioFrame},
		{name: "user speech started", event: NewUserSpeechStarted(), expected: KindUserSpeechStarted},
		{name: "user speech ended", event: NewUserSpeechEnded(), expected: KindUserSpeechEnded},
		{name: "user interim segment updated", event: NewUserTranscriptInterimSegmentUpdated("seg"), expected: KindUserTranscriptInterimSegmentUpdated},
		{name: "user interim updated", event: NewUserTranscriptInterimUpdated("text"), expected: KindUserTranscriptInterimUpdated},
		{name: "user transcript segment", event: NewUserTranscriptSegment("seg"), expected: KindUserTranscriptSegment},
		{name: "user transcript final", event: NewUserTranscriptFinal("text"), expected: KindUserTranscriptFinal},
		{name: "assistant response started", event: NewAssistantResponseStarted(), expected: KindAssistantResponseStarted},
		{name: "assistant response segment", event: NewAssistantResponseSegment("seg"), expected: KindAssistantResponseSegment},
		{name: "assistant response final", event: NewAssistantResponseFinal(), expected: KindAssistantResponseFinal},
		{name: "assistant response finalized", event: NewAssistantResponseFinalized("text"), expected: KindAssistantResponseFinalized},
		{name: "tool call started", event: NewToolCallStarted("id", "name", "{}"), expected: KindToolCallStarted},
		{name: "tool call completed", event: NewToolCallCompleted("id", "name", "ok"), expected: KindToolCallCompleted},
		{name: "tool call failed", event: NewToolCallFailed("id", "name", "boom"), expected: KindToolCallFailed},
		{name: "assistant speech frame", event: NewAssistantSpeechFrame([]byte{1}), expected: KindAssistantSpeechFrame},
		{name: "assistant speech mark generated", event: NewAssistantSpeechMarkGenerated("mark"), expected: KindAssistantSpeechMarkGenerated},
		{name: "assistant speech final", event: NewAssistantSpeechFinal(), expected: KindAssistantSpeechFinal},
		{name: "assistant playback started", event: NewAssistantPlaybackStarted(), expected: KindAssistantPlaybackStarted},
		{name: "assistant playback frame", event: NewAssistantPlaybackFrame([]byte{1}), expected: KindAssistantPlaybackFrame},
		{name: "assistant playback mark played", event: NewAssistantPlaybackMarkPlayed("mark-id", "text"), expected: KindAssistantPlaybackMarkPlayed},
		{name: "assistant playback transcript updated", event: NewAssistantPlaybackTranscriptUpdated("text"), expected: KindAssistantPlaybackTranscriptUpdated},
		{name: "assistant playback transcript segment", event: NewAssistantPlaybackTranscriptSegment("seg"), expected: KindAssistantPlaybackTranscriptSegment},
		{name: "assistant playback ended", event: NewAssistantPlaybackEnded("text"), expected: KindAssistantPlaybackEnded},
		{name: "turn started", event: NewTurnStarted("turn-id", "prompt"), expected: KindTurnStarted},
		{name: "turn completed", event: NewTurnCompleted("turn-id"), expected: KindTurnCompleted},
		{name: "turn failed", event: NewTurnFailed("turn-id", "error"), expected: KindTurnFailed},
		{name: "turn cancelled", event: NewTurnCancelled(), expected: KindTurnCancelled},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.event.Kind(); got != testCase.expected {
				t.Fatalf("expected kind %q, got %q", testCase.expected, got)
			}
		})
	}
}

func TestUserSpeechStartedAndEndedKindsAreDistinct(t *testing.T) {
	started := NewUserSpeechStarted()
	ended := NewUserSpeechEnded()

	if started.Kind() == ended.Kind() {
		t.Fatalf("expected speech started and speech ended kinds to differ, both were %q", started.Kind())
	}
}
