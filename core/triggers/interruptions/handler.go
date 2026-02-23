package interruptions

import (
	"context"
	"iter"
	"strings"
	"time"

	"github.com/koscakluka/ema-core/core/conversations"
	"github.com/koscakluka/ema-core/core/llms"
	coretriggers "github.com/koscakluka/ema-core/core/triggers"
)

type TriggerHandler struct {
	llm LLM
}

func NewTriggerHandlerWithStructuredPrompt(classificationLLM LLMWithStructuredPrompt) *TriggerHandler {
	return &TriggerHandler{llm: classificationLLM}
}

func NewTriggerHandlerWithGeneralPrompt(classificationLLM LLMWithGeneralPrompt) *TriggerHandler {
	return &TriggerHandler{llm: classificationLLM}
}

func (h *TriggerHandler) HandleTriggerV0(ctx context.Context, trigger llms.TriggerV0, conversation conversations.ActiveContextV0) iter.Seq2[llms.TriggerV0, error] {
	return func(yield func(llms.TriggerV0, error) bool) {
		trigger = normalizeTrigger(trigger)
		if shouldIgnoreTrigger(trigger) {
			return
		}

		switch trigger.(type) {
		case coretriggers.CallToolTrigger, coretriggers.CancelTurnTrigger, coretriggers.PauseTurnTrigger, coretriggers.UnpauseTurnTrigger:
			yield(trigger, nil)
			return
		}

		if h == nil || h.llm == nil {
			yield(trigger, nil)
			return
		}

		if conversation.ActiveTurn() == nil {
			yield(trigger, nil)
			return
		}

		interruption := llms.InterruptionV0{
			ID:     time.Now().UnixNano(),
			Source: trigger.String(),
		}

		if !yield(coretriggers.NewRecordInterruptionTrigger(interruption), nil) {
			return
		}

		history := conversation.History()
		if activeTurn := conversation.ActiveTurn(); activeTurn != nil {
			history = append(history, *activeTurn)
		}

		classified, err := classify(ctx, interruption, h.llm,
			WithHistory(history),
			WithTools(conversation.AvailableTools()),
		)
		if err != nil {
			if !yield(trigger, nil) {
				return
			}
			yield(nil, err)
			return
		}

		for _, interruptionTrigger := range resolveInterruptionAsTriggers(*classified, conversation) {
			if !yield(interruptionTrigger, nil) {
				return
			}
		}
		classified.Resolved = true
		yield(coretriggers.NewResolveInterruptionTrigger(classified.ID, classified.Type, true), nil)
	}
}

func shouldIgnoreTrigger(trigger llms.TriggerV0) bool {
	switch trigger.(type) {
	case coretriggers.SpeechStartedTrigger,
		coretriggers.SpeechEndedTrigger,
		coretriggers.InterimTranscriptionTrigger:
		return true
	default:
		return false
	}
}

func normalizeTrigger(trigger llms.TriggerV0) llms.TriggerV0 {
	if transcriptionTrigger, ok := trigger.(coretriggers.TranscriptionTrigger); ok {
		return coretriggers.NewTranscribedUserPromptTrigger(transcriptionTrigger.Transcript(), coretriggers.WithBase(transcriptionTrigger.BaseTrigger))
	}
	return trigger
}

func resolveInterruptionAsTriggers(interruption llms.InterruptionV0, conversation conversations.ActiveContextV0) []llms.TriggerV0 {
	switch interruptionType(interruption.Type) {
	case InterruptionTypeContinuation:
		prompt := continuationPrompt(interruption.Source, conversation)
		return []llms.TriggerV0{coretriggers.NewCancelTurnTrigger(), coretriggers.NewUserPromptTrigger(prompt)}
	case InterruptionTypeClarification:
		return []llms.TriggerV0{coretriggers.NewCancelTurnTrigger(), coretriggers.NewUserPromptTrigger(interruption.Source)}
	case InterruptionTypeCancellation:
		return []llms.TriggerV0{coretriggers.NewCancelTurnTrigger()}
	case InterruptionTypeIgnorable,
		InterruptionTypeRepetition,
		InterruptionTypeNoise:
		return []llms.TriggerV0{}
	case InterruptionTypeAction:
		return []llms.TriggerV0{coretriggers.NewCallToolWithPromptTrigger(interruption.Source)}
	case InterruptionTypeNewPrompt:
		return []llms.TriggerV0{coretriggers.NewUserPromptTrigger(interruption.Source)}
	default:
		return []llms.TriggerV0{coretriggers.NewUserPromptTrigger(interruption.Source)}
	}
}

func continuationPrompt(source string, conversation conversations.ActiveContextV0) string {
	lastPrompt := ""
	history := conversation.History()
	for i := len(history) - 1; i >= 0; i-- {
		if prompt, ok := promptFromTrigger(history[i].Trigger); ok {
			lastPrompt = prompt
			break
		}
	}

	if activeTurn := conversation.ActiveTurn(); activeTurn != nil {
		if prompt, ok := promptFromTrigger(activeTurn.Trigger); ok {
			lastPrompt = prompt
		}
	}

	if lastPrompt == "" {
		return source
	}
	return strings.TrimSpace(lastPrompt + " " + source)
}

func promptFromTrigger(trigger llms.TriggerV0) (string, bool) {
	if trigger == nil {
		return "", false
	}
	if userPrompt, ok := trigger.(coretriggers.UserPromptTrigger); ok {
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
