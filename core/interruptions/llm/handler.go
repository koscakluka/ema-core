package llm

import (
	"context"
	"fmt"
	"slices"

	emaContext "github.com/koscakluka/ema-core/core/context"
	"github.com/koscakluka/ema-core/core/interruptions"
	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/attribute"
)

type InterruptionHandlerWithStructuredPrompt struct {
	llm LLMWithStructuredPrompt
}

func NewInterruptionHandlerWithStructuredPrompt(classificationLLM LLMWithStructuredPrompt) *InterruptionHandlerWithStructuredPrompt {
	handler := &InterruptionHandlerWithStructuredPrompt{
		llm: classificationLLM,
	}
	return handler
}

func (h *InterruptionHandlerWithStructuredPrompt) HandleV0(prompt string, history []llms.Turn, tools []llms.Tool, orchestrator interruptions.OrchestratorV0) error {
	ctx := context.Background()
	interruption := &llms.InterruptionV0{ID: 0, Source: prompt}
	interruption, err := classify(ctx, *interruption, h.llm, WithHistory(llms.ToTurnsV1FromV0(history)), WithTools(tools))
	if err != nil {
		return err
	}
	_, err = respond(ctx, *interruption, orchestrator)
	return err
}

func (h *InterruptionHandlerWithStructuredPrompt) HandleV1(id int64, orchestrator interruptions.OrchestratorV0, tools []llms.Tool) (*llms.InterruptionV0, error) {
	ctx := context.Background()
	interruption := findInterruption(id, orchestrator.Turns())
	if interruption == nil {
		return nil, fmt.Errorf("interruption not found")
	}
	interruption, err := classify(ctx, *interruption, h.llm, WithHistory(getHistory(orchestrator.Turns())), WithTools(tools))
	if err != nil {
		return nil, err
	}
	// TODO: How do we handle interruption changing in the middle of resolving it?
	// activeInterruption := findInterruption(id, orchestrator.Turns())
	// if activeInterruption == nil {
	// 	return nil, fmt.Errorf("interruption not found after classification")
	// } else if activeInterruption.Resolved {
	// 	return nil, fmt.Errorf("interruption already resolved")
	// }

	return respond(ctx, *interruption, orchestrator)
}

func (h *InterruptionHandlerWithStructuredPrompt) HandleV2(ctx context.Context, id int64, orchestrator interruptions.OrchestratorV0, tools []llms.Tool) (*llms.InterruptionV0, error) {
	ctx, span := tracer.Start(ctx, "handling interruption")
	defer span.End()
	interruption := findInterruption(id, orchestrator.Turns())
	if interruption == nil {
		return nil, fmt.Errorf("interruption not found")
	}
	interruption, err := classify(ctx, *interruption, h.llm, WithHistory(getHistory(orchestrator.Turns())), WithTools(tools))
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("interruption.type", string(interruption.Type)))
	// TODO: How do we handle interruption changing in the middle of resolving it?
	// activeInterruption := findInterruption(id, orchestrator.Turns())
	// if activeInterruption == nil {
	// 	return nil, fmt.Errorf("interruption not found after classification")
	// } else if activeInterruption.Resolved {
	// 	return nil, fmt.Errorf("interruption already resolved")
	// }

	return respond(ctx, *interruption, orchestrator)
}

type LLMWithStructuredPrompt interface {
	PromptWithStructure(ctx context.Context, prompt string, outputSchema any, opts ...llms.StructuredPromptOption) error
}

type InterruptionHandlerWithGeneralPrompt struct {
	LLM
	llm LLMWithGeneralPrompt
}

func NewInterruptionHandlerWithGeneralPrompt(classificationLLM LLMWithGeneralPrompt) *InterruptionHandlerWithGeneralPrompt {
	handler := &InterruptionHandlerWithGeneralPrompt{
		llm: classificationLLM,
	}
	return handler
}

type LLMWithGeneralPrompt interface {
	LLM
	Prompt(ctx context.Context, prompt string, opts ...llms.GeneralPromptOption) (*llms.Message, error)
}

func (h *InterruptionHandlerWithGeneralPrompt) HandleV0(prompt string, history []llms.Turn, tools []llms.Tool, orchestrator interruptions.OrchestratorV0) error {
	ctx := context.Background()
	interruption := &llms.InterruptionV0{ID: 0, Source: prompt}
	interruption, err := classify(ctx, *interruption, h.llm, WithHistory(llms.ToTurnsV1FromV0(history)), WithTools(tools))
	if err != nil {
		return err
	}
	_, err = respond(ctx, *interruption, orchestrator)
	return err
}

func (h *InterruptionHandlerWithGeneralPrompt) HandleV1(id int64, orchestrator interruptions.OrchestratorV0, tools []llms.Tool) (*llms.InterruptionV0, error) {
	ctx := context.Background()
	interruption := findInterruption(id, orchestrator.Turns())
	if interruption == nil {
		return nil, fmt.Errorf("interruption not found")
	}
	interruption, err := classify(ctx, *interruption, h.llm, WithHistory(getHistory(orchestrator.Turns())), WithTools(tools))
	if err != nil {
		return nil, err
	}
	// TODO: How do we handle interruption changing in the middle of resolving it?
	// activeInterruption := findInterruption(id, orchestrator.Turns())
	// if activeInterruption == nil {
	// 	return nil, fmt.Errorf("interruption not found after classification")
	// } else if activeInterruption.Resolved {
	// 	return nil, fmt.Errorf("interruption already resolved")
	// }
	return respond(ctx, *interruption, orchestrator)
}

func (h *InterruptionHandlerWithGeneralPrompt) HandleV2(ctx context.Context, id int64, orchestrator interruptions.OrchestratorV0, tools []llms.Tool) (*llms.InterruptionV0, error) {
	ctx, span := tracer.Start(ctx, "handling interruption")
	defer span.End()
	interruption := findInterruption(id, orchestrator.Turns())
	if interruption == nil {
		return nil, fmt.Errorf("interruption not found")
	}
	interruption, err := classify(ctx, *interruption, h.llm, WithHistory(getHistory(orchestrator.Turns())), WithTools(tools))
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("interruption.type", string(interruption.Type)))
	// TODO: How do we handle interruption changing in the middle of resolving it?
	// activeInterruption := findInterruption(id, orchestrator.Turns())
	// if activeInterruption == nil {
	// 	return nil, fmt.Errorf("interruption not found after classification")
	// } else if activeInterruption.Resolved {
	// 	return nil, fmt.Errorf("interruption already resolved")
	// }
	return respond(ctx, *interruption, orchestrator)

}

type LLM any

func findInterruption(id int64, turns emaContext.TurnsV0) *llms.InterruptionV0 {
	for turn := range turns.RValues {
		for _, interruption := range turn.Interruptions {
			if interruption.ID == id {
				return &interruption
			}
		}
	}

	return nil
}

func getHistory(turns emaContext.TurnsV0) []llms.TurnV1 {
	var history []llms.Turn
	for turn := range turns.Values {
		history = append(history, turn)
	}
	historyV1 := llms.ToTurnsV1FromV0(history)
	slices.Reverse(historyV1)
	return historyV1
}
