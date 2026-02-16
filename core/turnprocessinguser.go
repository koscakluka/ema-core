package orchestration

import (
	"context"
	"log"

	"github.com/koscakluka/ema-core/core/events"
	"github.com/koscakluka/ema-core/core/speechtotext"
)

func (o *Orchestrator) initSST() {
	if o.speechToTextClient != nil {
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

		if err := o.speechToTextClient.Transcribe(context.TODO(), sttOptions...); err != nil {
			log.Fatalf("Failed to start transcribing: %v", err)
		}
	}
}
