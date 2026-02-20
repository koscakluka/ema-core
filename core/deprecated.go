package orchestration

import (
	"context"
	"fmt"
	"log"
	"slices"

	emaContext "github.com/koscakluka/ema-core/core/context"
	"github.com/koscakluka/ema-core/core/events"
	"github.com/koscakluka/ema-core/core/interruptions"
	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type InterruptionHandlerV0 interface {
	HandleV0(prompt string, turns []llms.Turn, tools []llms.Tool, orchestrator interruptions.OrchestratorV0) error
}

// Deprecated: use WithEventHandlerV0 instead.
func WithInterruptionHandlerV0(handler InterruptionHandlerV0) OrchestratorOption {
	return func(o *Orchestrator) {
		o.defaultEventHandler.interruptionHandlerV0 = handler
	}
}

type InterruptionHandlerV1 interface {
	HandleV1(id int64, orchestrator interruptions.OrchestratorV0, tools []llms.Tool) (*llms.InterruptionV0, error)
}

// Deprecated: use WithEventHandlerV0 instead.
func WithInterruptionHandlerV1(handler InterruptionHandlerV1) OrchestratorOption {
	return func(o *Orchestrator) {
		o.defaultEventHandler.interruptionHandlerV1 = handler
	}
}

type InterruptionHandlerV2 interface {
	HandleV2(ctx context.Context, id int64, orchestrator interruptions.OrchestratorV0, tools []llms.Tool) (*llms.InterruptionV0, error)
}

// Deprecated: use WithEventHandlerV0 instead.
func WithInterruptionHandlerV2(handler InterruptionHandlerV2) OrchestratorOption {
	return func(o *Orchestrator) {
		o.defaultEventHandler.interruptionHandlerV2 = handler
	}
}

// Deprecated: (since v0.0.17) use Orchestrator.SetAlwaysRecording instead.
func WithConfig(config *Config) OrchestratorOption {
	return func(o *Orchestrator) {
		if config == nil {
			return
		}

		o.SetAlwaysRecording(config.AlwaysRecording)
	}
}

// Config stores legacy orchestrator options.
//
// Deprecated: (since v0.0.17) use Orchestrator.SetAlwaysRecording.
type Config struct {
	AlwaysRecording bool
}

// WithLLM sets the LLM client for the orchestrator.
//
// Deprecated: (since v0.0.13) use WithStreamingLLM instead
func WithLLM(client LLMWithPrompt) OrchestratorOption {
	return func(o *Orchestrator) {
		o.llm.set(client)
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
		o.audioOutput.Set(client)
	}
}

// QueuePrompt immediately queues the prompt for processing after the current
// turn is finished. It bypasses the normal processing pipeline and can be useful
// for handling prompts that are sure to follow up after the current turn.
//
// Deprecated: (since v0.0.16)
func (o *Orchestrator) QueuePrompt(prompt string) {
	go func() {
		if ok := o.eventPlayer.Ingest(events.NewUserPromptEvent(prompt)); !ok {
			log.Printf("Warning: failed to queue prompt")
		}
	}()
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
	return llms.ToMessages(llms.ToTurnsV0FromV1(o.conversation.History()))
}

// Turns return llm Turns
//
// Deprecated: (since v0.0.15) use Orchestrator.ConversationV0 instead.
func (o *Orchestrator) Turns() emaContext.TurnsV0 {
	return &turns{conversation: &o.conversation}
}

// turns is a deprecated type that is used to provide backwards compatibility
// for the old TurnsV0 interface
//
// Deprecated: (since v0.0.15) use Orchestrator.ConversationV0 instead.
type turns struct {
	conversation *activeConversation
}

// Push is a deprecated method that is used to provide backwards compatibility
// for the old TurnsV0 interface. There is no new equivalent method at the
// moment.
//
// Deprecated: (since v0.0.15) no direct replacement is available.
func (t *turns) Push(turn llms.Turn) {
	t.conversation.appendTurns(llms.ToTurnsV1FromV0([]llms.Turn{turn})...)
}

// appendTurns is a deprecated method that is used to provide backwards compatibility
// for the old TurnsV0 interface.
//
// Deprecated: (since v0.0.15) no direct replacement is available.
func (t *activeConversation) appendTurns(turns ...llms.TurnV1) {
	t.legacyAppendTurns(turns...)
}

// Deprecated: internal compatibility shim for deprecated conversation mutation APIs.
func (t *activeConversation) legacyAppendTurns(turns ...llms.TurnV1) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.turns = append(t.turns, turns...)
}

// Deprecated: internal compatibility shim for deprecated conversation mutation APIs.
func (t *activeConversation) legacyResetActiveTurnIfMatches(activeTurn *activeTurn) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.activeTurn != activeTurn || t.activeTurn == nil {
		return false
	}

	t.activeTurn.Responses = nil
	t.activeTurn.ToolCalls = nil
	t.activeTurn.Interruptions = nil
	t.activeTurn.IsFinalised = false
	return true
}

// Deprecated: internal compatibility shim for deprecated conversation mutation APIs.
func (t *activeConversation) legacyClearActiveTurnIfMatches(activeTurn *activeTurn) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.activeTurn != activeTurn || t.activeTurn == nil {
		return false
	}

	t.activeTurn = nil
	return true
}

// Deprecated: internal compatibility shim for deprecated conversation mutation APIs.
func (t *activeConversation) legacyPopOldHistoryTurn() *llms.Turn {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.turns) == 0 {
		return nil
	}

	lastElementIdx := len(t.turns) - 1
	turn := t.turns[lastElementIdx]
	turns := llms.ToTurnsV0FromV1([]llms.TurnV1{turn})
	if len(turns) > 1 {
		t.turns[lastElementIdx].Responses = nil
		t.turns[lastElementIdx].ToolCalls = nil
		t.turns[lastElementIdx].Interruptions = nil
		t.turns[lastElementIdx].IsFinalised = false
		return &turns[1]
	}

	t.turns = t.turns[:lastElementIdx]
	return &turns[0]
}

// Deprecated: internal compatibility shim for deprecated conversation mutation APIs.
func (t *activeConversation) legacyDetachActiveTurn() *activeTurn {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.activeTurn == nil {
		return nil
	}

	activeTurn := t.activeTurn
	t.activeTurn = nil
	return activeTurn
}

// Deprecated: internal compatibility shim for deprecated conversation mutation APIs.
func (t *activeConversation) legacyPopHistoryTurn() *llms.TurnV1 {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.turns) == 0 {
		return nil
	}

	lastElementIdx := len(t.turns) - 1
	turn := t.turns[lastElementIdx]
	t.turns = t.turns[:lastElementIdx]
	return &turn
}

// Deprecated: internal compatibility shim for deprecated conversation mutation APIs.
func (t *activeConversation) legacyClearState() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.turns = nil
	t.activeTurn = nil
}

// Pop is a deprecated method that is used to provide backwards compatibility
// for the old TurnsV0 interface.
//
// Deprecated: (since v0.0.15) use Orchestrator.ConversationV0 instead.
func (t *turns) Pop() *llms.Turn {
	return t.conversation.popOld()
}

// popOld is a deprecated method that is used to provide backwards compatibility
// for the old TurnsV0 interface. There is no new equivalent method at the
// moment.
//
// Deprecated: (since v0.0.15) use [github.com/koscakluka/ema-core/core/conversations.ActiveContextV0] instead.
func (t *activeConversation) popOld() *llms.Turn {
	t.mu.RLock()
	activeTurn := t.activeTurn
	t.mu.RUnlock()
	if activeTurn != nil {
		if pipeline := t.activePipeline(); pipeline != nil {
			pipeline.Cancel()
		}
		turns := llms.ToTurnsV0FromV1([]llms.TurnV1{activeTurn.TurnV1})
		if len(turns) > 1 {
			t.legacyResetActiveTurnIfMatches(activeTurn)
			return &turns[1]
		}
		t.legacyClearActiveTurnIfMatches(activeTurn)
		return &turns[0]
	}

	return t.legacyPopOldHistoryTurn()
}

// Clear is a deprecated method that is used to provide backwards compatibility
// for the old TurnsV0 interface.
//
// Deprecated: (since v0.0.15) use Orchestrator.ConversationV0 instead.
func (t *turns) Clear() {
	t.conversation.Clear()
}

// Values is a deprecated method that is used to provide backwards compatibility
// for the old TurnsV0 interface.
//
// Deprecated: (since v0.0.15) use Orchestrator.ConversationV0 instead.
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

// RValues is a deprecated method that is used to provide backwards
// compatibility for the old TurnsV0 interface.
//
// Deprecated: (since v0.0.15) use Orchestrator.ConversationV0 instead.
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

// Pop removes the last turn from the stored turns, returns nil if empty
//
// Deprecated: (since v0.0.17) use [github.com/koscakluka/ema-core/core/conversations.ActiveContextV0] instead.
func (t *activeConversation) Pop() *llms.TurnV1 {
	if activeTurn := t.legacyDetachActiveTurn(); activeTurn != nil {
		turn := activeTurn.TurnV1
		if pipeline := t.activePipeline(); pipeline != nil {
			pipeline.Cancel()
		}
		return &turn
	}

	return t.legacyPopHistoryTurn()
}

// activePipeline returns the current response pipeline, or nil if there is none.
//
// Deprecated: (since v0.0.17)
// HACK: This is to allow backwards compatibility with the old
func (t *activeConversation) activePipeline() *responsePipeline {
	if t == nil || t.currentPipeline == nil {
		return nil
	}

	return t.currentPipeline()
}

// Clear removes all stored turns
//
// Deprecated: (since v0.0.17) use [github.com/koscakluka/ema-core/core/conversations.ActiveContextV0] instead.
func (t *activeConversation) Clear() {
	t.legacyClearState()
}

// Values is an iterator that goes over all the stored turns starting from the
// earliest towards the latest
//
// Deprecated: (since v0.0.17) use [github.com/koscakluka/ema-core/core/conversations.ActiveContextV0] instead.
func (t *activeConversation) Values(yield func(llms.TurnV1) bool) {
	for _, turn := range t.History() {
		if !yield(turn) {
			return
		}
	}
	if activeTurn := t.ActiveTurn(); activeTurn != nil {
		if !yield(*activeTurn) {
			return
		}
	}
}

// RValues is an iterator that goes over all the stored turns starting from the
// latest towards the earliest
//
// Deprecated: (since v0.0.17) use [github.com/koscakluka/ema-core/core/conversations.ActiveContextV0] instead.
func (t *activeConversation) RValues(yield func(llms.TurnV1) bool) {
	if activeTurn := t.ActiveTurn(); activeTurn != nil {
		if !yield(*activeTurn) {
			return
		}
	}
	// TODO: There should be a better way to do this than creating a new
	// method just for reversing the order
	for _, turn := range slices.Backward(t.History()) {
		if !yield(turn) {
			return
		}
	}
}

// ConversationV0 returns the legacy mutable conversation view.
//
// Deprecated: (since v0.0.17) use [github.com/koscakluka/ema-core/core/conversations.ActiveContextV0] via [EventHandlerV0.HandleV0] instead.
func (o *Orchestrator) ConversationV0() emaContext.ConversationV0 {
	return &o.conversation
}

// CallTool is a deprecated method that was used to execute tool calls
// from interruptions. It is no longer necessary to call this method.
//
// Deprecated: (since v0.0.17) events instead.
func (o *Orchestrator) CallTool(ctx context.Context, prompt string) error {
	ctx, span := tracer.Start(ctx, "call tool with prompt")
	defer span.End()
	runtimeLLM := o.llm.snapshot()
	_, err := runtimeLLM.generate(
		ctx,
		events.NewUserPromptEvent(prompt),
		o.conversation.History(),
		newTextBuffer(),
		func() bool { return o.currentResponsePipeline().IsCancelled() },
	)
	return err
}

// StartRecording starts recording audio input.
//
// Deprecated: (since v0.0.17) use RequestToCaptureAudio instead
func (o *Orchestrator) StartRecording() error { return o.RequestToCaptureAudio() }

// StopRecording stops recording audio input.
//
// Deprecated: (since v0.0.17) use StopRequestingToCaptureAudio instead
func (o *Orchestrator) StopRecording() error { return o.StopRequestingToCaptureAudio() }

// EnableAlwaysRecording enables continuous recording.
//
// Deprecated: (since v0.0.17) use EnableAlwaysCapturingAudio instead
func (o *Orchestrator) EnableAlwaysRecording(ctx context.Context) error {
	return o.EnableAlwaysCapturingAudio()
}

// DisableAlwaysRecording disables continuous recording.
//
// Deprecated: (since v0.0.17) use DisableAlwaysCapturingAudio instead
func (o *Orchestrator) DisableAlwaysRecording(ctx context.Context) error {
	return o.DisableAlwaysCapturingAudio()
}

// IsAlwaysRecording indicates whether the orchestrator is currently recording
// audio input.
//
// Deprecated: (since v0.0.17) use IsAlwaysCapturingAudio instead
func (o *Orchestrator) IsAlwaysRecording() bool { return o.IsAlwaysCapturingAudio() }

// SetAlwaysRecording enables or disables continuous recording of audio input.
//
// Deprecated: (since v0.0.17) use EnableAlwaysCapturingAudio or DisableAlwaysCapturingAudio instead
func (o *Orchestrator) SetAlwaysRecording(isAlwaysRecording bool) {
	var err error
	if isAlwaysRecording {
		err = o.EnableAlwaysCapturingAudio()
	} else {
		err = o.DisableAlwaysCapturingAudio()
	}

	if err != nil {
		recordedErr := fmt.Errorf("failed to set always recording to %t: %w", isAlwaysRecording, err)
		span := trace.SpanFromContext(o.baseContext)
		span.RecordError(recordedErr)
		span.SetStatus(codes.Error, recordedErr.Error())
	}
}

// SetSpeaking sets the orchestrator's speaking state. If the orchestrator is
// currently passing speech to audio output, it will stop passing speech to
// audio output, and vice versa.
//
// Deprecated: (since v0.0.17) use [Orchestrator.Mute] or [Orchestrator.Unmute]
func (o *Orchestrator) SetSpeaking(isSpeaking bool) {
	if isSpeaking {
		o.Unmute()
	} else {
		o.Mute()
	}
}
