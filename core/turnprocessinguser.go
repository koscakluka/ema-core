package orchestration

import (
	"context"
	"log"

	"github.com/koscakluka/ema-core/core/speechtotext"
	"github.com/koscakluka/ema-core/core/triggers"
)

func (o *Orchestrator) initSST() {
	if o.speechToTextClient != nil {
		sttOptions := []speechtotext.TranscriptionOption{
			speechtotext.WithSpeechStartedCallback(func() {
				go o.respondToTrigger(triggers.NewSpeechStartedTrigger())
			}),
			speechtotext.WithSpeechEndedCallback(func() {
				go o.respondToTrigger(triggers.NewSpeechEndedTrigger())
			}),
			speechtotext.WithInterimTranscriptionCallback(func(transcript string) {
				go o.respondToTrigger(triggers.NewInterimTranscriptionTrigger(transcript))
			}),
			speechtotext.WithTranscriptionCallback(func(transcript string) {
				go o.respondToTrigger(triggers.NewTranscriptionTrigger(transcript))
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
