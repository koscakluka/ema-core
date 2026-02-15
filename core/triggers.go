package orchestration

import (
	"fmt"
	"log"
	"time"

	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/core/triggers"
	"go.opentelemetry.io/otel/trace"
)

func (o *Orchestrator) respondToTrigger(trigger llms.TriggerV0) {
	switch t := trigger.(type) {
	case triggers.SpeechStartedTrigger:
		// TODO: Consider pausing on speech start
		// maybe with some wait time for interim transcript
		// or maybe pausing on interim transcript is enough
		if o.orchestrateOptions.onSpeakingStateChanged != nil {
			o.orchestrateOptions.onSpeakingStateChanged(true)
		}
		return
	case triggers.SpeechEndedTrigger:
		if o.orchestrateOptions.onSpeakingStateChanged != nil {
			o.orchestrateOptions.onSpeakingStateChanged(false)
		}
		return
	case triggers.InterimTranscriptionTrigger:
		// TODO: Start generating interruption here already
		// marking the ID will probably be required to keep track of it
		if o.orchestrateOptions.onInterimTranscription != nil {
			o.orchestrateOptions.onInterimTranscription(t.Transcript())
		}
		return
	case triggers.TranscriptionTrigger:
		if o.orchestrateOptions.onInterimTranscription != nil {
			o.orchestrateOptions.onInterimTranscription("")
		}
		if o.orchestrateOptions.onTranscription != nil {
			o.orchestrateOptions.onTranscription(t.Transcript())
		}

		trigger = triggers.NewTranscribedUserPromptTrigger(t.Transcript(), triggers.WithBase(t.BaseTrigger))
	}

	activeTurn := o.conversation.activeTurn
	if activeTurn == nil {
		o.queueTrigger(trigger)
		return
	}

	ctx := activeTurn.ctx
	span := trace.SpanFromContext(ctx)
	interruptionID := time.Now().UnixNano()
	if err := activeTurn.AddInterruption(llms.InterruptionV0{
		ID:     interruptionID,
		Source: trigger.String(),
	}); err != nil {
		span.RecordError(err)
		return
	}

	switch t := trigger.(type) {
	case triggers.UserPromptTrigger:
		// Just pass it through

	case triggers.CallToolTrigger:
		if t.Tool != nil {
			// TODO: This response should be recorded somewhere, probably in the
			// interruption, and might even warrant a response
			_, err := o.callTool(ctx, *t.Tool)
			if err != nil {
				span.RecordError(err)
			}
		} else {
			// TODO: There should be some kind of response somewhere, at least
			// recorded, probably in the interruption
			if err := o.CallTool(ctx, t.Prompt); err != nil {
				span.RecordError(err)
			}
		}
		return

	default:
		span.RecordError(fmt.Errorf("skipped processing trigger of unknown type: %T", trigger))
		return
	}

	if o.interruptionHandlerV2 != nil {
		if interruption, err := o.interruptionHandlerV2.HandleV2(ctx, interruptionID, o, o.tools); err != nil {
			log.Printf("Failed to handle interruption: %v", err)
		} else {
			o.conversation.updateInterruption(interruptionID, func(update *llms.InterruptionV0) {
				update.Type = interruption.Type
				update.Resolved = interruption.Resolved
			})
			return
		}
	} else if o.interruptionHandlerV1 != nil {
		if interruption, err := o.interruptionHandlerV1.HandleV1(interruptionID, o, o.tools); err != nil {
			log.Printf("Failed to handle interruption: %v", err)
		} else {
			o.conversation.updateInterruption(interruptionID, func(update *llms.InterruptionV0) {
				update.Type = interruption.Type
				update.Resolved = interruption.Resolved
			})
			return
		}
	} else if o.interruptionHandlerV0 != nil {
		if err := o.interruptionHandlerV0.HandleV0(trigger.String(), llms.ToTurnsV0FromV1(o.conversation.turns), o.tools, o); err != nil {
			log.Printf("Failed to handle interruption: %v", err)
		} else {
			o.conversation.updateInterruption(interruptionID, func(interruption *llms.InterruptionV0) {
				interruption.Resolved = true
			})
			return
		}
	}
	o.conversation.updateInterruption(interruptionID, func(interruption *llms.InterruptionV0) {
		interruption.Resolved = true
	})

}
