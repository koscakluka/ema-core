package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"

	emaContext "github.com/koscakluka/ema-core/core/context"
	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/internal/utils"
)

// WithLLM sets the LLM client for the orchestrator.
//
// Deprecated: (since v0.0.13) use WithStreamingLLM instead
func WithLLM(client LLMWithPrompt) OrchestratorOption {
	return func(o *Orchestrator) {
		o.llm = client
	}
}

// LLMWithPrompt
//
// Deprecated: (since v0.0.13) use LLMWithGeneralPrompt instead
type LLMWithPrompt interface {
	LLM
	Prompt(ctx context.Context, prompt string, opts ...llms.PromptOption) ([]llms.Message, error)
}

// WithInterruptionClassifier sets the interruption classifier that is used
// internally to classify interruptions types so orchestrator can respond to
// them.
//
// Deprecated: (since v0.0.13) use WithInterruptionHandler instead
func WithInterruptionClassifier(classifier InterruptionClassifier) OrchestratorOption {
	return func(o *Orchestrator) {
		o.interruptionClassifier = classifier
	}
}

// InterruptionClassifier
//
// Deprecated: (since v0.0.13) use InterruptionHandlers instead
type InterruptionClassifier interface {
	Classify(prompt string, history []llms.Message, opts ...ClassifyOption) (interruptionType, error)
}

// WithAudioOutput is a OrchestratorOption that sets the audio output client.
//
// Deprecated: (since v0.0.13) use WithAudioOutputV0 instead, we want to free up this option
func WithAudioOutput(client AudioOutputV0) OrchestratorOption {
	return func(o *Orchestrator) {
		o.audioOutput = client
	}
}

// Cancel is an alias for CancelTurn
//
// Deprecated: (since v0.0.13) use CancelTurn instead
func (o *Orchestrator) Cancel() {
	o.CancelTurn()
}

// Messages return old style llm messages
//
// Deprecated: (since v0.0.13) use Turns instead
func (o *Orchestrator) Messages() []llms.Message {
	return llms.ToMessages(llms.ToTurnsV0FromV1(o.conversation.turns))
}

// Turns return llm Turns
//
// Deprecated: (since v0.0.15) use Conversation instead
func (o *Orchestrator) Turns() emaContext.TurnsV0 {
	return &turns{conversation: &o.conversation}
}

// respondToInterruption
//
// Deprecated: (since v0.0.13) Use the InterruptionHandler interface instead of relaying on this
func (o *Orchestrator) respondToInterruption(prompt string, t interruptionType) (passthrough *string, err error) {
	// TODO: Take this out of the orchestrator and into a separate interuption
	// handler

	// TODO: Check if this is still relevant (do we still have an active prompt)
	switch t {
	case InterruptionTypeContinuation:
		o.Cancel()
		found := -1
		count := 0
		for turn := range o.Turns().RValues {
			if turn.Role == llms.TurnRoleUser {
				found = count
				break
			}
			count++
		}

		if found == -1 {
			return &prompt, nil
		}

		for range found {
			o.Turns().Pop()
		}

		lastUserTurn := o.Turns().Pop()
		if lastUserTurn != nil {
			return utils.Ptr(lastUserTurn.Content + " " + prompt), nil
		} else {
			return &prompt, nil
		}
	case InterruptionTypeClarification:
		o.Cancel()
		return &prompt, nil
		// TODO: Properly passthrough the modified prompt
	case InterruptionTypeCancellation:
		o.Cancel()
		return nil, nil
	case InterruptionTypeIgnorable,
		InterruptionTypeRepetition,
		InterruptionTypeNoise:
		return nil, nil
	case InterruptionTypeAction:
		switch o.llm.(type) {
		case LLMWithPrompt:
			if _, err := o.llm.(LLMWithPrompt).Prompt(context.TODO(), prompt,
				llms.WithForcedTools(o.tools...),
				llms.WithTurnsV1(o.conversation.turns...),
			); err != nil {
				// TODO: Retry?
				return nil, fmt.Errorf("failed to call tool LLM: %w", err)
			}
		case LLMWithGeneralPrompt:
			resp, err := o.llm.(LLMWithGeneralPrompt).Prompt(context.TODO(), prompt,
				llms.WithForcedTools(o.tools...),
				llms.WithTurnsV1(o.conversation.turns...),
			)
			if err != nil {
				// TODO: Retry?
				return nil, fmt.Errorf("failed to call tool LLM: %w", err)
			}

			for _, toolCall := range resp.ToolCalls {
				_, err := o.callTool(context.TODO(), toolCall)
				if err != nil {
					// TODO: Retry?
					return nil, fmt.Errorf("failed to call tool: %w", err)
				}
			}
		}
		return nil, nil
	case InterruptionTypeNewPrompt:
		// TODO: Consider interrupting the current prompt and asking to continue
		// with it before addressing the new prompt
		return &prompt, nil
	default:
		return &prompt, fmt.Errorf("unknown interruption type: %s", t)
	}
}

// SimpleInterruptionClassifier
//
// Deprecated: (since v0.0.13) use InterruptionHandlers instead
type SimpleInterruptionClassifier struct {
	llm   LLM
	tools []llms.Tool
}

// NewSimpleInterruptionClassifier creates a new SimpleInterruptionClassifier
// used to classify the type of interruption a prompt is.
//
// Deprecated:  (since v0.0.13) use InterruptionHandlers instead
func NewSimpleInterruptionClassifier(llm LLMWithPrompt, opts ...InterruptionClassifierOption) *SimpleInterruptionClassifier {
	classifier := &SimpleInterruptionClassifier{
		llm: llm,
	}
	for _, opt := range opts {
		opt(classifier)
	}
	return classifier
}

// InterruptionClassifierOption
//
// Deprecated: (since v0.0.13) use InterruptionHandlers instead
type InterruptionClassifierOption func(*SimpleInterruptionClassifier)

// ClassifierWithTools
//
// Deprecated: (since v0.0.13) use InterruptionHandlers instead
func ClassifierWithTools(tools []llms.Tool) InterruptionClassifierOption {
	return func(c *SimpleInterruptionClassifier) {
		c.tools = tools
	}
}

// ClassifierWithInterruptionLLM
//
// Deprecated: (since v0.0.13) use InterruptionHandlers instead
func ClassifierWithInterruptionLLM(llm InterruptionLLM) InterruptionClassifierOption {
	return func(c *SimpleInterruptionClassifier) {
		c.llm = llm
	}
}

// ClassifierWithGeneralPromptLLM
//
// Deprecated: (since v0.0.13) use InterruptionHandlers instead
func ClassifierWithGeneralPromptLLM(llm LLMWithGeneralPrompt) InterruptionClassifierOption {
	return func(c *SimpleInterruptionClassifier) {
		c.llm = llm
	}
}

const (
	// interruptionClassifierSystemPrompt
	//
	// Deprecated: (since v0.0.13) use InterruptionHandlers instead
	interruptionClassifierSystemPrompt = `You are a helpful assistant that can classify a prompt type of interruption to the conversation.

A conversation interruption can be classified as one of the following:
- continuation: The interruption is a continuation of the previous sentence/request (e.g. "Tell me about Star Wars.", "Ships design").
- cancellation: Anything that indicates that the response should not be finished. Only used if the interruption cannot be addressed by a listed tool.
- clarification: The interruption is a clarification or restatement of the previous instruction (e.g. "It's actually about the TV show, not the movie").
- ignorable: The interruption is ignorable and should not be responded to.
- repetition: The interruption is a repetition of the previous sentence/request.
- noise: The interruption is noise and should be ignored.
- action: The interruption is a addressable with a listed tool.
- new prompt: The interruption is a new prompt to be responded to that could not be understood as a continuation of the previous sentence

Only respond with the classification of the interruption as JSON: {"classification": "response"}

Accessible tools:
`
	// interruptionClassifierStructuredSystemPrompt
	//
	// Deprecated: (since v0.0.13) use InterruptionHandlers instead
	interruptionClassifierStructuredSystemPrompt = `You are a helpful assistant that can classify a prompt type of interruption to the conversation.

A conversation interruption can be classified as one of the following:
- continuation: The interruption is a continuation of the previous sentence/request (e.g. "Tell me about Star Wars.", "Ships design").
- cancellation: Anything that indicates that the response should not be finished. Only used if the interruption cannot be addressed by a listed tool.
- clarification: The interruption is a clarification or restatement of the previous instruction (e.g. "It's actually about the TV show, not the movie").
- ignorable: The interruption is ignorable and should not be responded to.
- repetition: The interruption is a repetition of the previous sentence/request.
- noise: The interruption is noise and should be ignored.
- action: The interruption is a addressable with a listed tool.
- new prompt: The interruption is a new prompt to be responded to that could not be understood as a continuation of the previous sentence

Accessible tools:
`
)

// Classification
//
// Deprecated: (since v0.0.13) use InterruptionHandlers instead
type Classification struct {
	Type string `json:"type" jsonschema:"title=Type,description=The type of interruption" enum:"continuation,clarification,cancellation,ignorable,repetition,noise,action,new prompt"`
}

// Classify
//
// Deprecated: (since v0.0.13) use InterruptionHandlers instead
func (c SimpleInterruptionClassifier) Classify(prompt string, history []llms.Message, opts ...ClassifyOption) (interruptionType, error) {
	options := ClassifyOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	classification := ""
	switch c.llm.(type) {
	case InterruptionLLM:
		systemPrompt := interruptionClassifierStructuredSystemPrompt
		for _, tool := range append(c.tools, options.Tools...) {
			systemPrompt += fmt.Sprintf("- %s: %s", tool.Function.Name, tool.Function.Description)
		}

		resp := Classification{}
		llm := c.llm.(InterruptionLLM)
		if err := llm.PromptWithStructure(context.TODO(), prompt,
			&resp,
			llms.WithSystemPrompt(systemPrompt),
			llms.WithMessages(history...),
		); err != nil {
			return "", err
		}

		classification = resp.Type

	case LLMWithGeneralPrompt:
		systemPrompt := interruptionClassifierSystemPrompt
		for _, tool := range append(c.tools, options.Tools...) {
			systemPrompt += fmt.Sprintf("- %s: %s", tool.Function.Name, tool.Function.Description)
		}

		response, _ := c.llm.(LLMWithGeneralPrompt).Prompt(context.TODO(), prompt,
			llms.WithSystemPrompt(systemPrompt),
			llms.WithMessages(history...),
		)

		if len(response.Content) == 0 {
			return "", fmt.Errorf("no response from interruption classifier")
		}

		var unmarshalledResponse struct {
			Classification string `json:"classification"`
		}
		if err := json.Unmarshal([]byte(response.Content), &unmarshalledResponse); err != nil {
			// TODO: Retry
			log.Printf("Failed to unmarshal interruption classification response: %v", err)
			return "", nil
		}
		classification = unmarshalledResponse.Classification

	case LLMWithPrompt:
		systemPrompt := interruptionClassifierSystemPrompt
		for _, tool := range append(c.tools, options.Tools...) {
			systemPrompt += fmt.Sprintf("- %s: %s", tool.Function.Name, tool.Function.Description)
		}

		response, _ := c.llm.(LLMWithPrompt).Prompt(context.TODO(), prompt,
			llms.WithSystemPrompt(systemPrompt),
			llms.WithMessages(history...),
		)

		if len(response) == 0 || len(response[0].Content) == 0 {
			return "", fmt.Errorf("no response from interruption classifier")
		}

		var unmarshalledResponse struct {
			Classification string `json:"classification"`
		}
		if err := json.Unmarshal([]byte(response[len(response)-1].Content), &unmarshalledResponse); err != nil {
			// TODO: Retry
			log.Printf("Failed to unmarshal interruption classification response: %v", err)
			return "", nil
		}
		classification = unmarshalledResponse.Classification

	}

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

// ClassifyOption
//
// Deprecated: (since v0.0.13) use InterruptionHandlers instead
type ClassifyOption func(*ClassifyOptions)

// ClassifyOptions
//
// Deprecated: (since v0.0.13) use InterruptionHandlers instead
type ClassifyOptions struct {
	Tools   []llms.Tool
	Context context.Context
}

// ClassifyWithTools
//
// Deprecated: (since v0.0.13) use InterruptionHandlers instead
func ClassifyWithTools(tools []llms.Tool) ClassifyOption {
	return func(o *ClassifyOptions) {
		o.Tools = tools
	}
}

// ClassifyWithContext
//
// Deprecated: (since v0.0.14) use InterruptionHandlers instead
func ClassifyWithContext(ctx context.Context) ClassifyOption {
	return func(o *ClassifyOptions) {
		o.Context = ctx
	}
}

// interruptionType
//
// Deprecated: (since v0.0.13) use InterruptionHandlers instead
type interruptionType string

const (
	InterruptionTypeContinuation  interruptionType = "continuation"
	InterruptionTypeClarification interruptionType = "clarification"
	InterruptionTypeCancellation  interruptionType = "cancellation"
	InterruptionTypeIgnorable     interruptionType = "ignorable"
	InterruptionTypeRepetition    interruptionType = "repetition"
	InterruptionTypeNoise         interruptionType = "noise"
	InterruptionTypeAction        interruptionType = "action"
	InterruptionTypeNewPrompt     interruptionType = "new prompt"
)

// InterruptionLLM
//
// Deprecated: (since v0.0.13) use InterruptionHandlers instead
type InterruptionLLM interface {
	PromptWithStructure(ctx context.Context, prompt string, outputSchema any, opts ...llms.StructuredPromptOption) error
}

// turns is a deprecated type that is used to provide backwards compatibility
// for the old TurnsV0 interface
//
// Deprecated: (since v0.0.15) use Conversation instead
type turns struct {
	conversation *Conversation
}

// Push is a deprecated method that is used to provide backwards compatibility
// for the old TurnsV0 interface. There is no new equivalent method at the
// moment.
//
// Deprecated: (since v0.0.15) use Conversation instead
func (t *turns) Push(turn llms.Turn) {
	t.conversation.turns = append(t.conversation.turns, llms.ToTurnsV1FromV0([]llms.Turn{turn})...)
}

func (t *turns) Pop() *llms.Turn {
	return t.conversation.popOld()
}

func (t *turns) Clear() {
	t.conversation.Clear()
}

func (t *turns) Values(yield func(llms.Turn) bool) {
	for turn := range t.conversation.Values {
		turns := llms.ToTurnsV0FromV1([]llms.TurnV1{turn})
		for _, turn := range turns {
			if !yield(turn) {
				return
			}
		}
	}
}

func (t *turns) RValues(yield func(llms.Turn) bool) {
	turnsV1 := []llms.TurnV1{}
	for turn := range t.conversation.Values {
		turnsV1 = append(turnsV1, turn)
	}
	turns := llms.ToTurnsV0FromV1(turnsV1)
	slices.Reverse(turns)
	for _, turn := range turns {
		if !yield(turn) {
			return
		}
	}
}
