package orchestration

import (
	"context"
	"fmt"
	"iter"
	"log"
	"time"

	"github.com/koscakluka/ema-core/core/conversations"
	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/core/triggers"
	"go.opentelemetry.io/otel/trace"
)

func (o *Orchestrator) ingestTrigger(trigger llms.TriggerV0) {
	ctx := o.currentActiveContext()
	for trigger, err := range o.triggerHandler.HandleTriggerV0(ctx, trigger, &o.conversation) {
		if err != nil {
			span := trace.SpanFromContext(o.currentActiveContext())
			span.RecordError(err)
			return
		}
		if trigger == nil {
			continue
		}

		// TODO: If this block grows further, replace this switch with one of:
		// 1) dispatch table: map event type -> handler func, so adding event types
		// stays local and avoids a large switch;
		// 2) reducer + effects: map events to explicit side effects first, then run
		// effects separately for easier testing;
		// 3) middleware pipeline: small chained handlers when we need logging,
		// retries, metrics, or other cross-cutting behavior around event handling.
		switch t := trigger.(type) {
		case triggers.CancelTurnTrigger:
			o.currentResponsePipeline().Cancel()
		case triggers.PauseTurnTrigger:
			o.currentResponsePipeline().Pause()
		case triggers.UnpauseTurnTrigger:
			o.currentResponsePipeline().Unpause()
		case triggers.RecordInterruptionTrigger:
			o.conversation.addInterruptionToActiveTurn(t.Interruption)
		case triggers.ResolveInterruptionTrigger:
			o.conversation.updateInterruption(t.ID, func(update *llms.InterruptionV0) {
				update.Type = t.Type
				update.Resolved = t.Resolved
			})
		case triggers.CallToolTrigger:
			if t.Tool != nil {
				_, err := o.callTool(ctx, *t.Tool)
				if err != nil {
					return
				}
			} else {
				if err := o.CallTool(ctx, t.Prompt); err != nil {
					return
				}
			}
			return
		default:
			if ok := o.triggerPlayer.Ingest(trigger); !ok {
				log.Printf("Warning: failed to enqueue trigger %T", trigger)
			}
		}
	}
}

type internalTriggerHandler struct {
	interruptionHandlerV0 InterruptionHandlerV0
	interruptionHandlerV1 InterruptionHandlerV1
	interruptionHandlerV2 InterruptionHandlerV2
	orchestrator          *Orchestrator
}

func (h *internalTriggerHandler) HandleTriggerV0(ctx context.Context, trigger llms.TriggerV0, conversation conversations.ActiveContextV0) iter.Seq2[llms.TriggerV0, error] {
	return func(yield func(llms.TriggerV0, error) bool) {
		trigger = h.normalizeTrigger(trigger)
		if h.shouldIgnoreTrigger(trigger) {
			return
		}

		switch trigger.(type) {
		case triggers.CallToolTrigger, triggers.CancelTurnTrigger, triggers.PauseTurnTrigger, triggers.UnpauseTurnTrigger:
			yield(trigger, nil)
			return
		}

		if activeTurn := conversation.ActiveTurn(); activeTurn == nil {
			yield(trigger, nil)
			return
		}

		interruption := h.newInterruption(trigger)
		if !yield(triggers.NewRecordInterruptionTrigger(*interruption), nil) {
			return
		}

		resolvedInterruption, interruptionTriggers, err := h.resolveInterruption(ctx, trigger, interruption, conversation)
		if err != nil {
			if !yield(trigger, nil) {
				return
			}
			yield(nil, err)
			return
		}

		for _, interruptionTrigger := range interruptionTriggers {
			if !yield(interruptionTrigger, nil) {
				return
			}
		}

		yield(triggers.NewResolveInterruptionTrigger(resolvedInterruption.ID, resolvedInterruption.Type, resolvedInterruption.Resolved), nil)
	}
}

func (h *internalTriggerHandler) shouldIgnoreTrigger(trigger llms.TriggerV0) bool {
	switch trigger.(type) {
	case triggers.SpeechStartedTrigger, // TODO: Consider pausing on speech start maybe with some wait time for interim transcript or maybe pausing on interim transcript is enough
		triggers.SpeechEndedTrigger,
		triggers.InterimTranscriptionTrigger: // TODO: Start generating interruption here already marking the ID will probably be required to keep track of it
		return true
	default:
		return false
	}
}

func (h *internalTriggerHandler) normalizeTrigger(trigger llms.TriggerV0) llms.TriggerV0 {
	if transcriptionTrigger, ok := trigger.(triggers.TranscriptionTrigger); ok {
		return triggers.NewTranscribedUserPromptTrigger(transcriptionTrigger.Transcript(), triggers.WithBase(transcriptionTrigger.BaseTrigger))
	}
	return trigger
}

func (h *internalTriggerHandler) newInterruption(trigger llms.TriggerV0) *llms.InterruptionV0 {
	return &llms.InterruptionV0{
		ID:     time.Now().UnixNano(),
		Source: trigger.String(),
	}
}

func (h *internalTriggerHandler) resolveInterruption(ctx context.Context, trigger llms.TriggerV0, interruption *llms.InterruptionV0, conversation conversations.ActiveContextV0) (*llms.InterruptionV0, []llms.TriggerV0, error) {
	availableTools := conversation.AvailableTools()
	if h.orchestrator == nil {
		return nil, nil, fmt.Errorf("internal event handler requires orchestrator")
	}

	if h.interruptionHandlerV2 != nil {
		resolved, err := h.interruptionHandlerV2.HandleV2(ctx, interruption.ID, h.orchestrator, availableTools)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to handle interruption: %w", err)
		}
		return resolved, nil, nil
	}

	if h.interruptionHandlerV1 != nil {
		resolved, err := h.interruptionHandlerV1.HandleV1(interruption.ID, h.orchestrator, availableTools)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to handle interruption: %w", err)
		}
		return resolved, nil, nil
	}

	if h.interruptionHandlerV0 != nil {
		err := h.interruptionHandlerV0.HandleV0(trigger.String(), llms.ToTurnsV0FromV1(conversation.History()), availableTools, h.orchestrator)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to handle interruption: %w", err)
		}
		interruption.Resolved = true
		return interruption, nil, nil
	}

	interruption.Resolved = true
	return interruption, nil, nil
}
