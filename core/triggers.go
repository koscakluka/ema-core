package orchestration

import (
	"fmt"
	"log"
	"time"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/trace"
)

type UserPromptTrigger struct {
	Prompt        string
	IsTranscribed bool
	Timestamp     time.Time
}

func (t UserPromptTrigger) String() string {
	return t.Prompt
}

func NewUserPromptTrigger(prompt string) UserPromptTrigger {
	return UserPromptTrigger{
		Prompt:        prompt,
		IsTranscribed: false,
		Timestamp:     time.Now(),
	}
}

func NewTranscribedUserPromptTrigger(prompt string) UserPromptTrigger {
	return UserPromptTrigger{
		Prompt:        prompt,
		IsTranscribed: true,
		Timestamp:     time.Now(),
	}
}

func (o *Orchestrator) respondToTrigger(trigger llms.TriggerV0) {
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
	case UserPromptTrigger:
		prompt := t.Prompt
		// TODO: Move this and note the change
		if t.IsTranscribed && o.orchestrateOptions.onTranscription != nil {
			o.orchestrateOptions.onTranscription(prompt)
		}

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
