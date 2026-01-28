package orchestration

import (
	"context"
	"log"
	"time"

	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/core/speechtotext"
	"github.com/koscakluka/ema-core/internal/utils"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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

				o.SendPrompt(transcript)
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

func (o *Orchestrator) processUserTurn(prompt string) {
	var interruptionID *int64
	ctx := context.Background()
	if activeTurn := o.turns.activeTurn; activeTurn != nil {
		interruptionID = utils.Ptr(time.Now().UnixNano())
		if err := activeTurn.addInterruption(llms.InterruptionV0{
			ID:     *interruptionID,
			Source: prompt,
		}); err != nil {
			interruptionID = nil
		} else {
			ctx = activeTurn.ctx
			span := trace.SpanFromContext(ctx)
			span.AddEvent("interruption", trace.WithAttributes(attribute.Int64("interruption.id", *interruptionID)))
		}
	}

	passthrough := &prompt
	if interruptionID != nil {
		if o.interruptionHandlerV2 != nil {
			if interruption, err := o.interruptionHandlerV2.HandleV2(ctx, *interruptionID, o, o.tools); err != nil {
				log.Printf("Failed to handle interruption: %v", err)
			} else {
				o.turns.updateInterruption(*interruptionID, func(update *llms.InterruptionV0) {
					update.Type = interruption.Type
					update.Resolved = interruption.Resolved
				})
				return
			}
		} else if o.interruptionHandlerV1 != nil {
			if interruption, err := o.interruptionHandlerV1.HandleV1(*interruptionID, o, o.tools); err != nil {
				log.Printf("Failed to handle interruption: %v", err)
			} else {
				o.turns.updateInterruption(*interruptionID, func(update *llms.InterruptionV0) {
					update.Type = interruption.Type
					update.Resolved = interruption.Resolved
				})
				return
			}
		} else if o.interruptionHandlerV0 != nil {
			if err := o.interruptionHandlerV0.HandleV0(prompt, o.turns.turns, o.tools, o); err != nil {
				log.Printf("Failed to handle interruption: %v", err)
			} else {
				o.turns.updateInterruption(*interruptionID, func(interruption *llms.InterruptionV0) {
					interruption.Resolved = true
				})
				return
			}
		} else if o.interruptionClassifier != nil {
			interruption, err := o.interruptionClassifier.Classify(prompt, llms.ToMessages(o.turns.turns), ClassifyWithTools(o.tools), ClassifyWithContext(ctx))
			if err != nil {
				// TODO: Retry?
				log.Printf("Failed to classify interruption: %v", err)
			} else {
				o.turns.updateInterruption(*interruptionID, func(i *llms.InterruptionV0) { i.Type = string(interruption) })
				passthrough, err = o.respondToInterruption(prompt, interruption)
				if err != nil {
					log.Printf("Failed to respond to interruption: %v", err)
				}
			}
		}
		o.turns.updateInterruption(*interruptionID, func(interruption *llms.InterruptionV0) {
			interruption.Resolved = true
		})
	}
	if passthrough != nil {
		o.queuePrompt(*passthrough)
	}
}

func (o *Orchestrator) queuePrompt(prompt string) {
	if o.orchestrateOptions.onTranscription != nil {
		o.orchestrateOptions.onTranscription(prompt)
	}
	o.transcripts <- promptQueueItem{content: prompt, queuedAt: time.Now()}
}

type promptQueueItem struct {
	content  string
	queuedAt time.Time
}
