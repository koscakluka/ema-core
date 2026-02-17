package orchestration

import (
	"fmt"

	"github.com/koscakluka/ema-core/core/events"
	"github.com/koscakluka/ema-core/core/speechtotext"
)

func (o *Orchestrator) initSST() error {
	if o.speechToTextClient == nil {
		return nil
	}

	sttOptions := []speechtotext.TranscriptionOption{
		speechtotext.WithSpeechStartedCallback(func() {
			go o.respondToEvent(events.NewSpeechStartedEvent())
		}),
		speechtotext.WithSpeechEndedCallback(func() {
			go o.respondToEvent(events.NewSpeechEndedEvent())
		}),
		speechtotext.WithInterimTranscriptionCallback(func(transcript string) {
			go o.respondToEvent(events.NewInterimTranscriptionEvent(transcript))
		}),
		speechtotext.WithTranscriptionCallback(func(transcript string) {
			go o.respondToEvent(events.NewTranscriptionEvent(transcript))
		}),
	}
	if o.audioInput != nil {
		sttOptions = append(sttOptions, speechtotext.WithEncodingInfo(o.audioInput.EncodingInfo()))
	}

	if err := o.speechToTextClient.Transcribe(o.baseContext, sttOptions...); err != nil {
		return fmt.Errorf("failed to start transcribing: %w", err)
	}

	return nil
}
