package orchestration

import (
	"context"
	"fmt"
	"iter"
	"log"
	"time"

	"github.com/koscakluka/ema-core/core/conversations"
	"github.com/koscakluka/ema-core/core/events"
	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/trace"
)

func (o *Orchestrator) respondToEvent(event llms.EventV0) {
	ctx := o.baseContext
	if activeTurnCtx := o.conversation.ActiveTurnContext(); activeTurnCtx != nil {
		ctx = activeTurnCtx
	}

	switch t := event.(type) {
	case events.SpeechStartedEvent:
		if o.orchestrateOptions.onSpeakingStateChanged != nil {
			o.orchestrateOptions.onSpeakingStateChanged(true)
		}
	case events.SpeechEndedEvent:
		if o.orchestrateOptions.onSpeakingStateChanged != nil {
			o.orchestrateOptions.onSpeakingStateChanged(false)
		}
	case events.InterimTranscriptionEvent:
		if o.orchestrateOptions.onInterimTranscription != nil {
			o.orchestrateOptions.onInterimTranscription(t.Transcript())
		}
	case events.TranscriptionEvent:
		if o.orchestrateOptions.onInterimTranscription != nil {
			o.orchestrateOptions.onInterimTranscription("")
		}
		if o.orchestrateOptions.onTranscription != nil {
			o.orchestrateOptions.onTranscription(t.Transcript())
		}
	}

	for event, err := range o.eventHandler.HandleV0(ctx, event, &o.conversation) {
		if err != nil {
			var span trace.Span
			if activeTurnCtx := o.conversation.ActiveTurnContext(); activeTurnCtx != nil {
				span = trace.SpanFromContext(activeTurnCtx)
			} else {
				span = trace.SpanFromContext(o.baseContext)
			}
			span.RecordError(err)
			return
		}
		if event == nil {
			continue
		}

		// TODO: If this block grows further, replace this switch with one of:
		// 1) dispatch table: map event type -> handler func, so adding event types
		// stays local and avoids a large switch;
		// 2) reducer + effects: map events to explicit side effects first, then run
		// effects separately for easier testing;
		// 3) middleware pipeline: small chained handlers when we need logging,
		// retries, metrics, or other cross-cutting behavior around event handling.
		switch t := event.(type) {
		case events.CancelTurnEvent:
			o.conversation.CancelActiveTurn()
		case events.PauseTurnEvent:
			o.conversation.pauseActiveTurn()
		case events.UnpauseTurnEvent:
			o.conversation.unpauseActiveTurn()
		case events.RecordInterruptionEvent:
			o.conversation.addInterruptionToActiveTurn(t.Interruption)
		case events.ResolveInterruptionEvent:
			o.conversation.updateInterruption(t.ID, func(update *llms.InterruptionV0) {
				update.Type = t.Type
				update.Resolved = t.Resolved
			})
		case events.CallToolEvent:
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
			if ok := o.conversation.Enqueue(event); !ok {
				log.Printf("Warning: failed to enqueue event %T", event)
			}
		}
	}
}

type internalEventHandler struct {
	interruptionHandlerV0 InterruptionHandlerV0
	interruptionHandlerV1 InterruptionHandlerV1
	interruptionHandlerV2 InterruptionHandlerV2
	orchestrator          *Orchestrator
}

func (h *internalEventHandler) HandleV0(ctx context.Context, event llms.EventV0, conversation conversations.ActiveContextV0) iter.Seq2[llms.EventV0, error] {
	return func(yield func(llms.EventV0, error) bool) {
		event = h.normalizeEvent(event)
		if h.shouldIgnoreEvent(event) {
			return
		}

		switch event.(type) {
		case events.CallToolEvent, events.CancelTurnEvent, events.PauseTurnEvent, events.UnpauseTurnEvent:
			yield(event, nil)
			return
		}

		if activeTurn := conversation.ActiveTurn(); activeTurn == nil {
			yield(event, nil)
			return
		}

		interruption := h.newInterruption(event)
		if !yield(events.NewRecordInterruptionEvent(*interruption), nil) {
			return
		}

		resolvedInterruption, interruptionEvents, err := h.resolveInterruption(ctx, event, interruption, conversation)
		if err != nil {
			if !yield(event, nil) {
				return
			}
			yield(nil, err)
			return
		}

		for _, interruptionEvent := range interruptionEvents {
			if !yield(interruptionEvent, nil) {
				return
			}
		}

		yield(events.NewResolveInterruptionEvent(resolvedInterruption.ID, resolvedInterruption.Type, resolvedInterruption.Resolved), nil)
	}
}

func (h *internalEventHandler) shouldIgnoreEvent(event llms.EventV0) bool {
	switch event.(type) {
	case events.SpeechStartedEvent, // TODO: Consider pausing on speech start maybe with some wait time for interim transcript or maybe pausing on interim transcript is enough
		events.SpeechEndedEvent,
		events.InterimTranscriptionEvent: // TODO: Start generating interruption here already marking the ID will probably be required to keep track of it
		return true
	default:
		return false
	}
}

func (h *internalEventHandler) normalizeEvent(event llms.EventV0) llms.EventV0 {
	if transcriptionEvent, ok := event.(events.TranscriptionEvent); ok {
		return events.NewTranscribedUserPromptEvent(transcriptionEvent.Transcript(), events.WithBase(transcriptionEvent.BaseEvent))
	}
	return event
}

func (h *internalEventHandler) newInterruption(event llms.EventV0) *llms.InterruptionV0 {
	return &llms.InterruptionV0{
		ID:     time.Now().UnixNano(),
		Source: event.String(),
	}
}

func (h *internalEventHandler) resolveInterruption(ctx context.Context, event llms.EventV0, interruption *llms.InterruptionV0, conversation conversations.ActiveContextV0) (*llms.InterruptionV0, []llms.EventV0, error) {
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
		err := h.interruptionHandlerV0.HandleV0(event.String(), llms.ToTurnsV0FromV1(conversation.History()), availableTools, h.orchestrator)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to handle interruption: %w", err)
		}
		interruption.Resolved = true
		return interruption, nil, nil
	}

	interruption.Resolved = true
	return interruption, nil, nil
}
