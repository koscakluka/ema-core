package orchestration

import (
	"context"
	"slices"

	emaContext "github.com/koscakluka/ema-core/core/context"
	"github.com/koscakluka/ema-core/core/llms"
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
	return llms.ToMessages(llms.ToTurnsV0FromV1(o.conversation.historySnapshot()))
}

// Turns return llm Turns
//
// Deprecated: (since v0.0.15) use Conversation instead
func (o *Orchestrator) Turns() emaContext.TurnsV0 {
	return &turns{conversation: &o.conversation}
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
	t.conversation.appendTurns(llms.ToTurnsV1FromV0([]llms.Turn{turn})...)
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
