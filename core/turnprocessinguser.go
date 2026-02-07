package orchestration

import (
	"context"
	"log"

	"github.com/koscakluka/ema-core/core/speechtotext"
)

func (o *Orchestrator) initSST() {
	if o.speechToTextClient != nil {
		sttOptions := []speechtotext.TranscriptionOption{
			speechtotext.WithSpeechStartedCallback(func() {
				// TODO: Consider pausing on speech start
				// maybe with some wait time for interim transcript
				// or maybe pausing on interim transcript is enough
				if o.orchestrateOptions.onSpeakingStateChanged != nil {
					o.orchestrateOptions.onSpeakingStateChanged(true)
				}
			}),
			speechtotext.WithSpeechEndedCallback(func() {
				if o.orchestrateOptions.onSpeakingStateChanged != nil {
					o.orchestrateOptions.onSpeakingStateChanged(false)
				}
			}),
			speechtotext.WithInterimTranscriptionCallback(func(transcript string) {
				// TODO: Start generating interruption here already
				// marking the ID will probably be required to keep track of it

				if o.orchestrateOptions.onInterimTranscription != nil {
					o.orchestrateOptions.onInterimTranscription(transcript)
				}
			}),
			speechtotext.WithTranscriptionCallback(func(transcript string) {
				if o.orchestrateOptions.onInterimTranscription != nil {
					o.orchestrateOptions.onInterimTranscription("")
				}

				go o.respondToTrigger(NewTranscribedUserPromptTrigger(transcript))
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
