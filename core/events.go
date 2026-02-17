package orchestration

import (
	"context"
	"fmt"
	"time"

	"github.com/koscakluka/ema-core/core/conversations"
	"github.com/koscakluka/ema-core/core/events"
	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/trace"
)

func (o *Orchestrator) respondToEvent(event llms.EventV0) {
	ctx := o.baseContext
	if activeTurn := o.conversation.activeTurn; activeTurn != nil {
		ctx = activeTurn.ctx
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

	// TODO: See what to do with the interruption here
	unhandledEvents, _, err := o.eventHandler.HandleV0(ctx, event, &orchestratorActiveContext{
		conversation: &o.conversation,
		tools:        o.tools,
	})
	if err != nil {
		var span trace.Span
		if activeTurn := o.conversation.activeTurn; activeTurn != nil {
			span = trace.SpanFromContext(activeTurn.ctx)
		} else {
			span = trace.SpanFromContext(o.baseContext)
		}
		span.RecordError(err)
		return
	}

	for _, event := range unhandledEvents {
		switch t := event.(type) {
		case events.CallToolEvent:
			if t.Tool != nil {
				// TODO: This response should be recorded somewhere, probably in the
				// interruption, and might even warrant a response
				_, err := o.callTool(ctx, *t.Tool)
				if err != nil {
					return
				}
			} else {
				// TODO: There should be some kind of response somewhere, at least
				// recorded, probably in the interruption
				if err := o.CallTool(ctx, t.Prompt); err != nil {
					return
				}
			}
			return
		default:
			o.queueEvent(event)
		}
	}

}

type orchestratorActiveContext struct {
	conversation *Conversation
	tools        []llms.Tool
}

func (c *orchestratorActiveContext) History() []llms.TurnV1 {
	if c == nil || c.conversation == nil {
		return nil
	}
	history := make([]llms.TurnV1, len(c.conversation.turns))
	copy(history, c.conversation.turns)
	return history
}

func (c *orchestratorActiveContext) ActiveTurn() *llms.TurnV1 {
	if c == nil || c.conversation == nil || c.conversation.activeTurn == nil {
		return nil
	}
	return &c.conversation.activeTurn.TurnV1
}

func (c *orchestratorActiveContext) AvailableTools() []llms.Tool {
	if c == nil {
		return nil
	}
	tools := make([]llms.Tool, len(c.tools))
	copy(tools, c.tools)
	return tools
}

type internalEventHandler struct {
	interruptionHandlerV0 InterruptionHandlerV0
	interruptionHandlerV1 InterruptionHandlerV1
	interruptionHandlerV2 InterruptionHandlerV2
	orchestrator          *Orchestrator
}

func (h *internalEventHandler) HandleV0(ctx context.Context, event llms.EventV0, conversation conversations.ActiveContextV0) ([]llms.EventV0, *llms.InterruptionV0, error) {
	event = h.normalizeEvent(event)
	if h.shouldIgnoreEvent(event) {
		return []llms.EventV0{}, nil, nil
	}

	if _, isCallTool := event.(events.CallToolEvent); isCallTool {
		return []llms.EventV0{event}, nil, nil
	}

	if activeTurn := conversation.ActiveTurn(); activeTurn == nil {
		return []llms.EventV0{event}, nil, nil
	}

	interruption := h.newInterruption(event)
	h.orchestrator.conversation.activeTurn.Interruptions = append(h.orchestrator.conversation.activeTurn.Interruptions, *interruption)

	resolvedInterruption, err := h.resolveInterruption(ctx, event, interruption, conversation)
	if err != nil {
		return []llms.EventV0{event}, nil, err
	}

	return nil, resolvedInterruption, nil
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

func (h *internalEventHandler) resolveInterruption(ctx context.Context, event llms.EventV0, interruption *llms.InterruptionV0, conversation conversations.ActiveContextV0) (*llms.InterruptionV0, error) {
	availableTools := conversation.AvailableTools()

	if h.interruptionHandlerV2 != nil {
		resolved, err := h.interruptionHandlerV2.HandleV2(ctx, interruption.ID, h.orchestrator, availableTools)
		if err != nil {
			return nil, fmt.Errorf("failed to handle interruption: %w", err)
		}
		h.updateInterruption(interruption.ID, resolved.Type, resolved.Resolved)
		return resolved, nil
	}

	if h.interruptionHandlerV1 != nil {
		resolved, err := h.interruptionHandlerV1.HandleV1(interruption.ID, h.orchestrator, availableTools)
		if err != nil {
			return nil, fmt.Errorf("failed to handle interruption: %w", err)
		}
		h.updateInterruption(interruption.ID, resolved.Type, resolved.Resolved)
		return resolved, nil
	}

	if h.interruptionHandlerV0 != nil {
		err := h.interruptionHandlerV0.HandleV0(event.String(), llms.ToTurnsV0FromV1(conversation.History()), availableTools, h.orchestrator)
		if err != nil {
			return nil, fmt.Errorf("failed to handle interruption: %w", err)
		}
		h.updateInterruption(interruption.ID, interruption.Type, true)
		interruption.Resolved = true
		return interruption, nil
	}

	h.updateInterruption(interruption.ID, interruption.Type, true)
	interruption.Resolved = true
	return interruption, nil
}

func (h *internalEventHandler) updateInterruption(id int64, typ string, resolved bool) {
	h.orchestrator.conversation.updateInterruption(id, func(update *llms.InterruptionV0) {
		update.Type = typ
		update.Resolved = resolved
	})
}
