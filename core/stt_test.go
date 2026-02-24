package orchestration

import (
	"context"
	"testing"

	"github.com/koscakluka/ema-core/core/audio"
	events "github.com/koscakluka/ema-core/core/events"
	"github.com/koscakluka/ema-core/core/speechtotext"
)

func TestSpeechToTextStartEmitsEvents(t *testing.T) {
	sttClient := &speechToTextClientStub{
		transcribe: func(opts speechtotext.TranscriptionOptions) {
			if opts.SpeechStartedCallback == nil {
				t.Fatalf("expected speech-start callback to be configured")
			}
			if opts.SpeechEndedCallback == nil {
				t.Fatalf("expected speech-end callback to be configured")
			}
			if opts.PartialInterimTranscriptionCallback == nil {
				t.Fatalf("expected partial interim callback to be configured")
			}
			if opts.InterimTranscriptionCallback == nil {
				t.Fatalf("expected interim callback to be configured")
			}
			if opts.PartialTranscriptionCallback == nil {
				t.Fatalf("expected partial transcription callback to be configured")
			}
			if opts.TranscriptionCallback == nil {
				t.Fatalf("expected transcription callback to be configured")
			}

			opts.SpeechStartedCallback()
			opts.PartialInterimTranscriptionCallback("hel")
			opts.InterimTranscriptionCallback("hel")
			opts.PartialTranscriptionCallback("hello")
			opts.TranscriptionCallback("hello")
			opts.SpeechEndedCallback()
		},
	}

	runtime := newSpeechToText(sttClient)

	states := []bool{}
	partialInterim := []string{}
	interim := []string{}
	partialTranscriptions := []string{}
	transcriptions := []string{}

	runtime.SetEventEmitter(func(event events.Event) {
		switch typedEvent := event.(type) {
		case events.UserSpeechStarted:
			states = append(states, true)
		case events.UserSpeechEnded:
			states = append(states, false)
		case events.UserTranscriptInterimSegmentUpdated:
			partialInterim = append(partialInterim, typedEvent.Segment)
		case events.UserTranscriptInterimUpdated:
			interim = append(interim, typedEvent.Transcript)
		case events.UserTranscriptSegment:
			partialTranscriptions = append(partialTranscriptions, typedEvent.Segment)
		case events.UserTranscriptFinal:
			transcriptions = append(transcriptions, typedEvent.Transcript)
		}
	})

	encodingInfo := audio.GetDefaultEncodingInfo()
	if err := runtime.Start(context.Background(), &encodingInfo); err != nil {
		t.Fatalf("expected start to succeed, got %v", err)
	}

	if len(states) != 2 || !states[0] || states[1] {
		t.Fatalf("expected speaking states [true false], got %v", states)
	}

	if len(interim) != 2 || interim[0] != "hel" || interim[1] != "" {
		t.Fatalf("expected interim callbacks [\"hel\" \"\"], got %v", interim)
	}

	if len(partialInterim) != 2 || partialInterim[0] != "hel" || partialInterim[1] != "" {
		t.Fatalf("expected partial interim callbacks [\"hel\" \"\"], got %v", partialInterim)
	}

	if len(partialTranscriptions) != 1 || partialTranscriptions[0] != "hello" {
		t.Fatalf("expected partial transcription callback [\"hello\"], got %v", partialTranscriptions)
	}

	if len(transcriptions) != 1 || transcriptions[0] != "hello" {
		t.Fatalf("expected transcription callback [\"hello\"], got %v", transcriptions)
	}
}

func TestSpeechToTextInvokeTranscriptionClearsInterimBeforeFinal(t *testing.T) {
	runtime := newSpeechToText(nil)

	type observedEvent struct {
		kind       events.Kind
		transcript string
	}
	observed := []observedEvent{}
	runtime.SetEventEmitter(func(event events.Event) {
		switch typedEvent := event.(type) {
		case events.UserTranscriptInterimSegmentUpdated:
			observed = append(observed, observedEvent{kind: typedEvent.Kind(), transcript: typedEvent.Segment})
		case events.UserTranscriptInterimUpdated:
			observed = append(observed, observedEvent{kind: typedEvent.Kind(), transcript: typedEvent.Transcript})
		case events.UserTranscriptFinal:
			observed = append(observed, observedEvent{kind: typedEvent.Kind(), transcript: typedEvent.Transcript})
		}
	})

	runtime.invokeTranscription("final")

	if len(observed) != 3 {
		t.Fatalf("expected three events (partial interim clear, interim clear, transcription), got %d", len(observed))
	}

	if observed[0].kind != events.KindUserTranscriptInterimSegmentUpdated || observed[0].transcript != "" {
		t.Fatalf("expected first event to clear partial interim transcription, got %+v", observed[0])
	}
	if observed[1].kind != events.KindUserTranscriptInterimUpdated || observed[1].transcript != "" {
		t.Fatalf("expected second event to clear interim transcription, got %+v", observed[1])
	}
	if observed[2].kind != events.KindUserTranscriptFinal || observed[2].transcript != "final" {
		t.Fatalf("expected third event to emit final transcription, got %+v", observed[2])
	}
}

type speechToTextClientStub struct {
	transcribe func(opts speechtotext.TranscriptionOptions)
}

func (stub *speechToTextClientStub) Transcribe(_ context.Context, opts ...speechtotext.TranscriptionOption) error {
	transcriptionOptions := speechtotext.TranscriptionOptions{}
	for _, opt := range opts {
		opt(&transcriptionOptions)
	}

	if stub.transcribe != nil {
		stub.transcribe(transcriptionOptions)
	}

	return nil
}

func (stub *speechToTextClientStub) SendAudio([]byte) error {
	return nil
}
