package llm

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/koscakluka/ema-core/core/llms"
)

//go:embed classifierInstr.tmpl
var interruptionClassifierSystemPrompt string

//go:embed classifierSturctInsttr.tmpl
var interruptionClassifierStructuredSystemPrompt string

type Classification struct {
	Type string `json:"type" jsonschema:"title=Type,description=The type ofinterruption,enum=continuation,enum=clarification,enum=cancellation,enum=ignorable,enum=repetition,enum=noise,enum=action,enum=new prompt "`
}

func classify(ctx context.Context, interruption llms.InterruptionV0, llm LLM, opts ...ClassifyOption) (*llms.InterruptionV0, error) {
	options := ClassifyOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	switch llm.(type) {
	case LLMWithStructuredPrompt:
		systemPrompt := interruptionClassifierStructuredSystemPrompt
		for _, tool := range options.Tools {
			systemPrompt += fmt.Sprintf("- %s: %s", tool.Function.Name, tool.Function.Description)
		}

		resp := Classification{}
		if err := llm.(LLMWithStructuredPrompt).PromptWithStructure(ctx, interruption.Source,
			&resp,
			llms.WithSystemPrompt(systemPrompt),
			llms.WithTurnsV1(options.History...),
		); err != nil {
			// TODO: Retry?
			return &interruption, err
		}

		interruptionType, err := toInterruptionType(resp.Type)
		if err != nil {
			return nil, err
		}
		interruption.Type = string(interruptionType)
		return &interruption, nil

	case LLMWithGeneralPrompt:
		systemPrompt := interruptionClassifierSystemPrompt
		for _, tool := range options.Tools {
			systemPrompt += fmt.Sprintf("- %s: %s", tool.Function.Name, tool.Function.Description)
		}

		response, err := llm.(LLMWithGeneralPrompt).Prompt(ctx, interruption.Source,
			llms.WithSystemPrompt(systemPrompt),
			llms.WithTurnsV1(options.History...),
		)
		if err != nil {
			return &interruption, fmt.Errorf("failed to prompt interruption classifier: %w", err)
		}

		if len(response.Content) == 0 {
			return nil, fmt.Errorf("no response from interruption classifier")
		}

		var unmarshalledResponse struct {
			Classification string `json:"classification"`
		}
		if err := json.Unmarshal([]byte(response.Content), &unmarshalledResponse); err != nil {
			// TODO: Retry
			return nil, fmt.Errorf("failed to unmarshal interruption classification response: %w", err)
		}

		interruptionType, err := toInterruptionType(unmarshalledResponse.Classification)
		if err != nil {
			return nil, err
		}
		interruption.Type = string(interruptionType)
		return &interruption, nil
	}

	return nil, fmt.Errorf("unknown llm type")
}

func toInterruptionType(classification string) (interruptionType, error) {
	switch classification {
	case "continuation":
		return InterruptionTypeContinuation, nil
	case "clarification":
		return InterruptionTypeClarification, nil
	case "cancellation":
		return InterruptionTypeCancellation, nil
	case "ignorable":
		return InterruptionTypeIgnorable, nil
	case "repetition":
		return InterruptionTypeRepetition, nil
	case "noise":
		return InterruptionTypeNoise, nil
	case "action":
		return InterruptionTypeAction, nil
	case "new prompt":
		return InterruptionTypeNewPrompt, nil
	default:
		return "", fmt.Errorf("unknown interruption type: %s", classification)
	}
}

type ClassifyOption func(*ClassifyOptions)

type ClassifyOptions struct {
	History []llms.TurnV1
	Tools   []llms.Tool
}

func WithTools(tools []llms.Tool) ClassifyOption {
	return func(o *ClassifyOptions) {
		o.Tools = tools
	}
}

func WithHistory(history []llms.TurnV1) ClassifyOption {
	return func(o *ClassifyOptions) {
		o.History = history
	}
}
