package orchestration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"log"

	events "github.com/koscakluka/ema-core/core/events"
	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/core/triggers"
	"github.com/koscakluka/ema-core/internal/utils"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Orchestrator struct {
	baseContext  context.Context
	conversation activeConversation

	closeOnce sync.Once

	// audioInput is the input facade used to normalize capture behavior.
	audioInput audioInput
	// speechToText is the STT facade used to handle optional client wiring.
	speechToText speechToText
	llm          llm
	textToSpeech textToSpeech
	audioOutput  audioOutput
	speechPlayer speechPlayer

	triggerHandler TriggerHandlerV0
	// defaultTriggerHandler is the internal trigger handler used to handle incoming
	// triggers if no other handler is configured.
	//
	// TODO: Remove defaultTriggerHandler once we remove the interruption handlers
	// probably on minor release
	defaultTriggerHandler internalTriggerHandler

	triggerPlayer    *triggerPlayer
	responsePipeline atomic.Pointer[responsePipeline]

	// IsRecording indicates whether the orchestrator is currently recording
	// audio input.
	//
	// Deprecated: (since v0.0.17) use [Orchestrator.IsCapturingAudio] instead
	IsRecording bool
	// IsSpeaking indicates whether the orchestrator is currently passing speech
	// to audio output.
	//
	// Deprecated: (sinde v0.0.17) use [Orchestrator.IsMuted] instead
	IsSpeaking bool
}

func NewOrchestrator(opts ...OrchestratorOption) *Orchestrator {
	isRecording := false
	isSpeaking := false

	o := &Orchestrator{
		IsRecording: isRecording,
		IsSpeaking:  isSpeaking,

		baseContext: context.Background(),

		audioInput:   *newAudioInput(nil),
		speechToText: *newSpeechToText(nil),
		llm:          newLLM(),
		textToSpeech: *newTextToSpeech(nil /* isMuted */, !isSpeaking),
		audioOutput:  *newAudioOutput(nil),
		speechPlayer: *newSpeechPlayer(),

		triggerPlayer: newTriggerPlayer(),
	}
	// TODO: Move up once pipeline is removed from the constructor
	o.conversation = newConversation(o.currentResponsePipeline, o.llm.availableTools)

	// TODO: Remove defaultTriggerHandler once we remove the interruption handlers
	// probably on minor release
	o.defaultTriggerHandler.orchestrator = o
	o.triggerHandler = &o.defaultTriggerHandler

	for _, opt := range opts {
		opt(o)
	}

	return o
}

func (o *Orchestrator) Close() {
	o.closeOnce.Do(func() {
		o.triggerPlayer.Stop()
		o.currentResponsePipeline().Cancel()

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

		o.triggerPlayer.AwaitDone()
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
	// TODO: Rename this to StartConversation which will start a new conversation
	// with everything setup
	// It is also probably worth it to invest into a builder pattern instead of
	// using the options, including the callbacks and then finish with calling
	// the StartConversation method. It will be clearer to the caller and
	// it will allow reusing the orchestrator. But we need to have the contract
	// say that if there is a running conversation, it will be cancelled.
	// and new one started from scratch.
	// Additionally, we need to allow populating the conversation with
	// tools and history.
	// This method will probably need a EndConversation method to nicely clean
	// up the conversation if the user choosed to do so.

	if !o.triggerPlayer.CanIngest() {
		log.Println("Warning: orchestrator already closed, skipping Orchestrate")
		return
	}

	orchestrateOptions := OrchestrateOptions{}
	for _, opt := range opts {
		opt(&orchestrateOptions)
	}
	emitEvent := newCallbackEventEmitter(orchestrateOptions)

	o.baseContext = ctx
	o.llm.SetEventEmitter(emitEvent)
	o.textToSpeech.SetEventEmitter(emitEvent)
	o.speechPlayer.SetEventEmitter(emitEvent)
	o.speechToText.SetEventEmitter(o.composeSTTEventEmitter(emitEvent))
	o.audioInput.SetEventEmitter(o.composeAudioInputEventEmitter(emitEvent))
	if started := o.triggerPlayer.StartLoop(o.baseContext, func(ctx context.Context, trigger llms.TriggerV0) error {
		var turnErr error
		var activeTurn *activeTurn

		pipeline := newResponsePipeline(o.llm.snapshot(), o.textToSpeech.Snapshot(), o.speechPlayer.Snapshot(), o.audioOutput.Snapshot(),
			emitEvent,
		)
		if !o.responsePipeline.CompareAndSwap(nil, pipeline) {
			return fmt.Errorf("active turn already in progress")
		}
		defer o.responsePipeline.CompareAndSwap(pipeline, nil)

		activeTurn, turnErr = o.conversation.startNewTurn(trigger)
		if turnErr != nil {
			return turnErr
		}

		emitEvent(events.NewTurnStarted(activeTurn.TurnV1.ID, trigger.String()))
		defer func() {
			if turnErr != nil {
				emitEvent(events.NewTurnFailed(activeTurn.TurnV1.ID, turnErr.Error()))
			}
		}()

		activeTurn.TurnV1, turnErr = pipeline.Run(ctx, activeTurn, o.conversation.History())
		if turnErr != nil {
			// TODO: We should do something more reasonable here
			if err2 := o.conversation.finaliseTurn(activeTurn.TurnV1); err2 != nil {
				turnErr = errors.Join(turnErr, fmt.Errorf("failed to finalise turn: %w", err2))
			}
			turnErr = fmt.Errorf("failed to run pipeline: %w", turnErr)
			return turnErr
		}

		interruptionTypes := []string{}
		for _, interruption := range activeTurn.Interruptions {
			interruptionTypes = append(interruptionTypes, interruption.Type)
		}
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(attribute.StringSlice("assistant_turn.interruptions", interruptionTypes))
		span.SetAttributes(attribute.Int("assistant_turn.queued_triggers", o.triggerPlayer.queuedTriggerCount()))

		if err := o.conversation.finaliseTurn(activeTurn.TurnV1); err != nil {
			turnErr = fmt.Errorf("failed to finalise turn: %w", err)
			return turnErr
		}

		if !activeTurn.TurnV1.IsCancelled() {
			emitEvent(events.NewTurnCompleted(activeTurn.TurnV1.ID))
		}
		return nil
	}); started {
		go func() {
			<-ctx.Done()
			o.Close()
		}()
	}

	if err := o.speechToText.Start(o.baseContext, utils.Ptr(o.audioInput.EncodingInfo())); err != nil {
		recordedErr := fmt.Errorf("failed to initialize speech-to-text: %w", err)
		span := trace.SpanFromContext(o.baseContext)
		span.RecordError(recordedErr)
		span.SetStatus(codes.Error, recordedErr.Error())
	}

	o.audioInput.Start(o.baseContext)
}

func (o *Orchestrator) composeSTTEventEmitter(emitEvent eventEmitter) eventEmitter {
	if emitEvent == nil {
		emitEvent = noopEventEmitter
	}

	return func(event events.Event) {
		emitEvent(event)

		switch typedEvent := event.(type) {
		case events.UserSpeechStarted:
			go o.ingestTrigger(triggers.NewSpeechStartedTrigger())
		case events.UserSpeechEnded:
			go o.ingestTrigger(triggers.NewSpeechEndedTrigger())
		case events.UserTranscriptInterimUpdated:
			if typedEvent.Transcript != "" {
				go o.ingestTrigger(triggers.NewInterimTranscriptionTrigger(typedEvent.Transcript))
			}
		case events.UserTranscriptFinal:
			go o.ingestTrigger(triggers.NewTranscriptionTrigger(typedEvent.Transcript))
		}
	}
}

func (o *Orchestrator) composeAudioInputEventEmitter(emitEvent eventEmitter) eventEmitter {
	if emitEvent == nil {
		emitEvent = noopEventEmitter
	}

	return func(event events.Event) {
		emitEvent(event)

		if inputAudio, ok := event.(events.UserAudioFrame); ok {
			o.speechToText.SendAudio(inputAudio.Audio)
		}
	}
}

// ConversationV1 returns a point-in-time snapshot of conversation state.
func (o *Orchestrator) ConversationV1() ConversationV1 {
	return o.conversation.Snapshot()
}

func (o *Orchestrator) HandleTrigger(trigger llms.TriggerV0) { o.ingestTrigger(trigger) }
func (o *Orchestrator) SendPrompt(prompt string) {
	o.ingestTrigger(triggers.NewUserPromptTrigger(prompt))
}
func (o *Orchestrator) CancelTurn()  { o.ingestTrigger(triggers.NewCancelTurnTrigger()) }
func (o *Orchestrator) PauseTurn()   { o.ingestTrigger(triggers.NewPauseTurnTrigger()) }
func (o *Orchestrator) UnpauseTurn() { o.ingestTrigger(triggers.NewUnpauseTurnTrigger()) }

func (o *Orchestrator) SendAudio(audio []byte) error { return o.speechToText.SendAudio(audio) }

// IsMuted indicates whether the orchestrator is currently passing speech to
// audio output. True means the orchestrator is currently not passing speech to
// audio output.
func (o *Orchestrator) IsMuted() bool { return o.textToSpeech.IsMuted() }

// Mute stops passing speech to audio output.
func (o *Orchestrator) Mute() {
	o.IsSpeaking = false
	o.textToSpeech.Mute()
	o.currentResponsePipeline().StopSpeaking()
}

// Unmute resumes passing speech to audio output.
func (o *Orchestrator) Unmute() {
	o.IsSpeaking = true
	o.textToSpeech.Unmute()
}

// IsCapturingAudio indicates whether the orchestrator is currently capturing
// audio input. It provides no reason why the orchestrator is capturing audio.
func (o *Orchestrator) IsCapturingAudio() bool { return o.audioInput.IsCapturing() }

// IsAlwaysCapturingAudio indicates whether the orchestrator is currently
// always capturing audio input.
func (o *Orchestrator) IsAlwaysCapturingAudio() bool { return o.audioInput.IsAlwaysRecording() }

// EnableAlwaysCapturingAudio enables continuous capturing of audio input.
func (o *Orchestrator) EnableAlwaysCapturingAudio() error {
	return o.audioInput.EnableAlwaysCapture(o.baseContext)
}

// DisableAlwaysCapturingAudio disables continuous capturing of audio input.
func (o *Orchestrator) DisableAlwaysCapturingAudio() error {
	return o.audioInput.DisableAlwaysCapture(o.baseContext)
}

// IsRequestedToCaptureAudio indicates whether the orchestrator is currently
// requested to capture audio input._
func (o *Orchestrator) IsRequestedToCaptureAudio() bool { return o.audioInput.ShouldCapture() }

// RequestToCaptureAudio requests to capture audio input.
//
// This will have no effect if the orchestrator is always or already capturing
// audio.
func (o *Orchestrator) RequestToCaptureAudio() error {
	// TODO: Remove this assignment once on next minor release
	o.IsRecording = true
	return o.audioInput.RequestCapture(o.baseContext)
}

// StopRequestingToCaptureAudio stops requesting to capture audio input.
//
// This will have no effect if the orchestrator is always capturing audio or
// isn't capturing audio at the moment.
func (o *Orchestrator) StopRequestingToCaptureAudio() error {
	// TODO: Remove this assignment once on next minor release
	o.IsRecording = false
	return o.audioInput.ReleaseCapture(o.baseContext)
}
