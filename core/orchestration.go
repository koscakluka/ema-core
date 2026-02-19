package orchestration

import (
	"context"
	"fmt"
	"sync"

	"log"

	"github.com/koscakluka/ema-core/core/events"
	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/internal/utils"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Orchestrator struct {
	IsRecording bool
	IsSpeaking  bool

	conversation activeConversation

	closeOnce sync.Once
	runtime   *conversationRuntime

	// speechToText is the STT facade used to handle optional client wiring.
	speechToText speechToText
	// audioInput is the input facade used to normalize capture behavior.
	audioInput          audioInput
	eventHandler        EventHandlerV0
	defaultEventHandler internalEventHandler
	orchestrateOptions  OrchestrateOptions
	baseContext         context.Context
}

func NewOrchestrator(opts ...OrchestratorOption) *Orchestrator {
	isSpeaking := false
	runtime := newConversationRuntime()
	runtime.setSpeaking(isSpeaking)

	o := &Orchestrator{
		IsRecording: false,
		IsSpeaking:  isSpeaking,
		baseContext: context.Background(),
		defaultEventHandler: internalEventHandler{
			interruptionHandlerV0: nil,
			interruptionHandlerV1: nil,
			interruptionHandlerV2: nil,
			orchestrator:          nil,
		},
		runtime:      runtime,
		conversation: newConversation(runtime),
	}

	o.audioInput = *newAudioInput(nil, func(audio []byte) {
		if o.orchestrateOptions.onInputAudio != nil {
			o.orchestrateOptions.onInputAudio(audio)
		}

		o.speechToText.SendAudio(audio)
	})
	o.defaultEventHandler.orchestrator = o
	o.eventHandler = &o.defaultEventHandler

	for _, opt := range opts {
		opt(o)
	}

	return o
}

func (o *Orchestrator) Close() {
	o.closeOnce.Do(func() {
		o.conversation.End()

		if err := o.audioInput.Close(); err != nil {
			recordedErr := fmt.Errorf("failed to close audio input: %w", err)
			span := trace.SpanFromContext(o.baseContext)
			span.RecordError(recordedErr)
			span.SetStatus(codes.Error, recordedErr.Error())
		}

		if err := o.speechToText.Close(o.baseContext); err != nil {
			recordedErr := fmt.Errorf("failed to close speech-to-text client: %w", err)
			span := trace.SpanFromContext(o.baseContext)
			span.RecordError(recordedErr)
			span.SetStatus(codes.Error, recordedErr.Error())
		}

		o.conversation.AwaitCompletion()
	})
}

// Orchestrate starts the orchestrator that waits for any triggers to respond to
//
// ctx is used as a base context for any agent and tool calls, allowing for
// cancellation
//
// Contract: call Orchestrate at most once per orchestrator instance.
// Repeated or concurrent calls are unsupported and may race while runtime
// callbacks/options are being reconfigured.
// TODO: Enforce this contract with a hard runtime guard (single-start gate).
func (o *Orchestrator) Orchestrate(ctx context.Context, opts ...OrchestrateOption) {
	if o.runtime.isClosed() {
		log.Println("Warning: orchestrator already closed, skipping Orchestrate")
		return
	}

	o.orchestrateOptions = OrchestrateOptions{}
	for _, opt := range opts {
		opt(&o.orchestrateOptions)
	}

	o.baseContext = ctx
	o.runtime.configure(ctx, runtimeCallbacks{
		onResponse:     o.orchestrateOptions.onResponse,
		onResponseEnd:  o.orchestrateOptions.onResponseEnd,
		onAudio:        o.orchestrateOptions.onAudio,
		onAudioEnded:   o.orchestrateOptions.onAudioEnded,
		onCancellation: o.orchestrateOptions.onCancellation,
	})

	if started := o.conversation.Start(); started {
		go func() {
			<-ctx.Done()
			o.Close()
		}()
	}

	if err := o.speechToText.start(
		o.baseContext,
		speechToTextCallbacks{
			onSpeechStarted:        func() { go o.respondToEvent(events.NewSpeechStartedEvent()) },
			onSpeechEnded:          func() { go o.respondToEvent(events.NewSpeechEndedEvent()) },
			onInterimTranscription: func(transcript string) { go o.respondToEvent(events.NewInterimTranscriptionEvent(transcript)) },
			onTranscription:        func(transcript string) { go o.respondToEvent(events.NewTranscriptionEvent(transcript)) },
		},
		utils.Ptr(o.audioInput.EncodingInfo()),
	); err != nil {
		recordedErr := fmt.Errorf("failed to initialize speech-to-text: %w", err)
		span := trace.SpanFromContext(o.baseContext)
		span.RecordError(recordedErr)
		span.SetStatus(codes.Error, recordedErr.Error())
	}
	o.audioInput.Start(o.baseContext)
}

// ConversationV1 returns a point-in-time snapshot of conversation state.
func (o *Orchestrator) ConversationV1() ConversationV1 {
	return o.conversation.Snapshot()
}

func (o *Orchestrator) IsAlwaysRecording() bool { return o.audioInput.IsAlwaysRecording() }
func (o *Orchestrator) SetAlwaysRecording(isAlwaysRecording bool) {
	var err error
	if isAlwaysRecording {
		err = o.EnableAlwaysRecording(o.baseContext)
	} else {
		err = o.DisableAlwaysRecording(o.baseContext)
	}

	if err != nil {
		recordedErr := fmt.Errorf("failed to set always recording to %t: %w", isAlwaysRecording, err)
		span := trace.SpanFromContext(o.baseContext)
		span.RecordError(recordedErr)
		span.SetStatus(codes.Error, recordedErr.Error())
	}
}

func (o *Orchestrator) EnableAlwaysRecording(ctx context.Context) error {
	return o.audioInput.EnableAlwaysCapture(ctx)
}

func (o *Orchestrator) DisableAlwaysRecording(ctx context.Context) error {
	return o.audioInput.DisableAlwaysCapture(ctx)
}

func (o *Orchestrator) Handle(event llms.EventV0) { o.respondToEvent(event) }
func (o *Orchestrator) SendPrompt(prompt string)  { o.respondToEvent(events.NewUserPromptEvent(prompt)) }
func (o *Orchestrator) CancelTurn()               { o.respondToEvent(events.NewCancelTurnEvent()) }
func (o *Orchestrator) PauseTurn()                { o.respondToEvent(events.NewPauseTurnEvent()) }
func (o *Orchestrator) UnpauseTurn()              { o.respondToEvent(events.NewUnpauseTurnEvent()) }

func (o *Orchestrator) SendAudio(audio []byte) error { return o.speechToText.SendAudio(audio) }

func (o *Orchestrator) SetSpeaking(isSpeaking bool) {
	o.IsSpeaking = isSpeaking
	o.runtime.setSpeaking(isSpeaking)
	if !isSpeaking {
		o.conversation.stopSpeakingActiveTurn()
	}
}

func (o *Orchestrator) StartRecording() error {
	o.IsRecording = true
	return o.audioInput.RequestCapture(o.baseContext)
}

func (o *Orchestrator) StopRecording() error {
	o.IsRecording = false
	return o.audioInput.ReleaseCapture(o.baseContext)
}

func (o *Orchestrator) CallTool(ctx context.Context, prompt string) error {
	ctx, span := tracer.Start(ctx, "call tool with prompt")
	defer span.End()
	_, err := o.runtime.llm.generate(
		ctx,
		events.NewUserPromptEvent(prompt),
		o.conversation.History(),
		newTextBuffer(),
		func() bool { return o.conversation.IsActiveTurnCancelled() },
	)
	return err
}
