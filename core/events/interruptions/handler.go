package interruptions

import (
	"context"
	"strings"
	"time"

	"github.com/koscakluka/ema-core/core/conversations"
	coreevents "github.com/koscakluka/ema-core/core/events"
	"github.com/koscakluka/ema-core/core/llms"
)

type EventHandler struct {
	llm LLM
}

func NewEventHandlerWithStructuredPrompt(classificationLLM LLMWithStructuredPrompt) *EventHandler {
	return &EventHandler{llm: classificationLLM}
}

func NewEventHandlerWithGeneralPrompt(classificationLLM LLMWithGeneralPrompt) *EventHandler {
	return &EventHandler{llm: classificationLLM}
}

func (h *EventHandler) HandleV0(ctx context.Context, event llms.EventV0, conversation conversations.ActiveContextV0) ([]llms.EventV0, error) {
	event = normalizeEvent(event)
	if shouldIgnoreEvent(event) {
		return []llms.EventV0{}, nil
	}

	if _, isCallTool := event.(coreevents.CallToolEvent); isCallTool {
		return []llms.EventV0{event}, nil
	}

	if h == nil || h.llm == nil {
		return []llms.EventV0{event}, nil
	}

	if conversation.ActiveTurn() == nil {
		return []llms.EventV0{event}, nil
	}

	interruption := llms.InterruptionV0{
		ID:     time.Now().UnixNano(),
		Source: event.String(),
	}

	history := conversation.History()
	if activeTurn := conversation.ActiveTurn(); activeTurn != nil {
		history = append(history, *activeTurn)
	}

	classified, err := classify(ctx, interruption, h.llm,
		WithHistory(history),
		WithTools(conversation.AvailableTools()),
	)

	eventsOut := []llms.EventV0{coreevents.NewRecordInterruptionEvent(interruption)}
	if err != nil {
		return append(eventsOut, event), err
	}

	for _, interruptionEvent := range resolveInterruptionAsEvents(*classified, conversation) {
		eventsOut = append(eventsOut, interruptionEvent)
	}
	classified.Resolved = true
	eventsOut = append(eventsOut, coreevents.NewResolveInterruptionEvent(classified.ID, classified.Type, true))

	return eventsOut, nil
}

func shouldIgnoreEvent(event llms.EventV0) bool {
	switch event.(type) {
	case coreevents.SpeechStartedEvent,
		coreevents.SpeechEndedEvent,
		coreevents.InterimTranscriptionEvent:
		return true
	default:
		return false
	}
}

func normalizeEvent(event llms.EventV0) llms.EventV0 {
	if transcriptionEvent, ok := event.(coreevents.TranscriptionEvent); ok {
		return coreevents.NewTranscribedUserPromptEvent(transcriptionEvent.Transcript(), coreevents.WithBase(transcriptionEvent.BaseEvent))
	}
	return event
}

func resolveInterruptionAsEvents(interruption llms.InterruptionV0, conversation conversations.ActiveContextV0) []llms.EventV0 {
	switch interruptionType(interruption.Type) {
	case InterruptionTypeContinuation:
		prompt := continuationPrompt(interruption.Source, conversation)
		return []llms.EventV0{coreevents.NewCancelTurnEvent(), coreevents.NewUserPromptEvent(prompt)}
	case InterruptionTypeClarification:
		return []llms.EventV0{coreevents.NewCancelTurnEvent(), coreevents.NewUserPromptEvent(interruption.Source)}
	case InterruptionTypeCancellation:
		return []llms.EventV0{coreevents.NewCancelTurnEvent()}
	case InterruptionTypeIgnorable,
		InterruptionTypeRepetition,
		InterruptionTypeNoise:
		return []llms.EventV0{}
	case InterruptionTypeAction:
		return []llms.EventV0{coreevents.NewCallToolWithPromptEvent(interruption.Source)}
	case InterruptionTypeNewPrompt:
		return []llms.EventV0{coreevents.NewUserPromptEvent(interruption.Source)}
	default:
		return []llms.EventV0{coreevents.NewUserPromptEvent(interruption.Source)}
	}
}

func continuationPrompt(source string, conversation conversations.ActiveContextV0) string {
	lastPrompt := ""
	history := conversation.History()
	for i := len(history) - 1; i >= 0; i-- {
		if prompt, ok := promptFromEvent(history[i].Event); ok {
			lastPrompt = prompt
			break
		}
	}

	if activeTurn := conversation.ActiveTurn(); activeTurn != nil {
		if prompt, ok := promptFromEvent(activeTurn.Event); ok {
			lastPrompt = prompt
		}
	}

	if lastPrompt == "" {
		return source
	}
	return strings.TrimSpace(lastPrompt + " " + source)
}

func promptFromEvent(event llms.EventV0) (string, bool) {
	if event == nil {
		return "", false
	}
	if userPrompt, ok := event.(coreevents.UserPromptEvent); ok {
		return userPrompt.Prompt, true
	}
	return "", false
}

type LLMWithStructuredPrompt interface {
	PromptWithStructure(ctx context.Context, prompt string, outputSchema any, opts ...llms.StructuredPromptOption) error
}

type LLMWithGeneralPrompt interface {
	Prompt(ctx context.Context, prompt string, opts ...llms.GeneralPromptOption) (*llms.Message, error)
}

type LLM any
