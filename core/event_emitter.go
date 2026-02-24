package orchestration

import events "github.com/koscakluka/ema-core/core/events"

type eventEmitter func(events.Event)

func noopEventEmitter(events.Event) {}

func newCallbackEventEmitter(opts OrchestrateOptions) eventEmitter {
	return func(event events.Event) {
		switch typedEvent := event.(type) {
		case events.UserAudioFrame:
			if opts.onInputAudio != nil {
				opts.onInputAudio(typedEvent.Audio)
			}
		case events.UserSpeechStarted:
			if opts.onSpeakingStateChanged != nil {
				opts.onSpeakingStateChanged(true)
			}
		case events.UserSpeechEnded:
			if opts.onSpeakingStateChanged != nil {
				opts.onSpeakingStateChanged(false)
			}
		case events.UserTranscriptInterimUpdated:
			if opts.onInterimTranscription != nil {
				opts.onInterimTranscription(typedEvent.Transcript)
			}
		case events.UserTranscriptInterimSegmentUpdated:
			if opts.onPartialInterimTranscription != nil {
				opts.onPartialInterimTranscription(typedEvent.Segment)
			}
		case events.UserTranscriptSegment:
			if opts.onPartialTranscription != nil {
				opts.onPartialTranscription(typedEvent.Segment)
			}
		case events.UserTranscriptFinal:
			if opts.onTranscription != nil {
				opts.onTranscription(typedEvent.Transcript)
			}
		case events.AssistantResponseSegment:
			if opts.onResponse != nil {
				opts.onResponse(typedEvent.Segment)
			}
		case events.AssistantResponseFinal:
			if opts.onResponseEnd != nil {
				opts.onResponseEnd()
			}
		case events.AssistantSpeechFrame:
			if opts.onAudio != nil {
				opts.onAudio(typedEvent.Audio)
			}
		case events.AssistantPlaybackEnded:
			if opts.onAudioEnded != nil {
				opts.onAudioEnded(typedEvent.Transcript)
			}
		case events.AssistantPlaybackTranscriptUpdated:
			if opts.onSpokenText != nil {
				opts.onSpokenText(typedEvent.Transcript)
			}
		case events.AssistantPlaybackTranscriptSegment:
			if opts.onSpokenTextDelta != nil {
				opts.onSpokenTextDelta(typedEvent.Segment)
			}
		case events.TurnCancelled:
			if opts.onCancellation != nil {
				opts.onCancellation()
			}
		}
	}
}
