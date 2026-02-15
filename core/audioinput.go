package orchestration

import (
	"context"
	"log"
)

// initAudioCapture initializes and start audio capture if it makes sense
// (i.e. if we have a capture and speech to text client, and necessary settings
// are set)
func (o *Orchestrator) initAudioInput() {
	if o.audioInput != nil && o.speechToTextClient != nil {
		switch o.audioInput.(type) {
		case AudioInputFine:
			if o.config.AlwaysRecording {
				go func() {
					if err := o.startCapture(); err != nil {
						log.Printf("Failed to start audio input: %v", err)
					}
				}()
			}
		default:
			go o.captureAudio(context.TODO())
		}
	} else if o.audioInput != nil && o.speechToTextClient == nil {
		log.Println("Warning: skip starting input audio stream: audio input set but speech to text client is not set")
	}
}

// sendAudio sends audio to the speech to text client if one is set
func (o *Orchestrator) sendAudio(audio []byte) error {
	if o.orchestrateOptions.onInputAudio != nil {
		o.orchestrateOptions.onInputAudio(audio)
	}

	if o.speechToTextClient == nil {
		log.Println("Warning: SendAudio called but speech to text client is not set")
		return nil
	}

	if o.IsRecording || o.config.AlwaysRecording {
		return o.speechToTextClient.SendAudio(audio)
	}

	return nil
}

// startCapture start the audio capture for AudioInputFine, does nothing
// if AudioInputFine interface is not satisfied
func (o *Orchestrator) startCapture() error {
	if fineAudioInput, ok := o.audioInput.(AudioInputFine); ok {
		if err := fineAudioInput.StartCapture(context.TODO(), func(audio []byte) {
			if err := o.SendAudio(audio); err != nil {
				log.Printf("Failed to send audio to speech to text client: %v", err)
			}
		}); err != nil {
			return err
		}
	}

	return nil
}

// stopCapture stops the audio capture for AudioInputFine, does nothing
// if AudioInputFine interface is not satisfied
func (o *Orchestrator) stopCapture() error {
	if fineAudioInput, ok := o.audioInput.(AudioInputFine); ok {
		if err := fineAudioInput.StopCapture(); err != nil {
			return err
		}
	}
	return nil
}

// streamInputAudio captures the audio from the audio input and sends it to the
// speech to text client
func (o *Orchestrator) captureAudio(ctx context.Context) {
	if err := o.audioInput.Stream(ctx, func(audio []byte) {
		if err := o.SendAudio(audio); err != nil {
			log.Printf("Failed to send audio to speech to text client: %v", err)
		}
	}); err != nil {
		log.Printf("Failed to start audio input streaming: %v", err)
	}
}
