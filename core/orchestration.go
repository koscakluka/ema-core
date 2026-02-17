package orchestration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"log"

	emaContext "github.com/koscakluka/ema-core/core/context"
	"github.com/koscakluka/ema-core/core/events"
	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/core/speechtotext"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Orchestrator struct {
	IsRecording bool
	IsSpeaking  bool

	conversation Conversation

	eventQueue chan eventQueueItem
	closeCh    chan struct{}

	assistantLoopDone    chan struct{}
	assistantLoopOnce    sync.Once
	assistantLoopStarted atomic.Bool
	closeOnce            sync.Once

	tools []llms.Tool

	llm                 LLM
	speechToTextClient  SpeechToText
	textToSpeechClient  textToSpeech
	audioInput          AudioInput
	audioOutput         audioOutput
	eventHandler        EventHandlerV0
	defaultEventHandler internalEventHandler

	orchestrateOptions OrchestrateOptions
	config             *Config

	baseContext context.Context
}

func NewOrchestrator(opts ...OrchestratorOption) *Orchestrator {
	o := &Orchestrator{
		IsRecording:       false,
		IsSpeaking:        false,
		eventQueue:        make(chan eventQueueItem, 10), // TODO: Figure out good valiues for this
		closeCh:           make(chan struct{}),
		assistantLoopDone: make(chan struct{}),
		config:            &Config{AlwaysRecording: true},
		baseContext:       context.Background(),
		defaultEventHandler: internalEventHandler{
			interruptionHandlerV0: nil,
			interruptionHandlerV1: nil,
			interruptionHandlerV2: nil,
			orchestrator:          nil,
		},
	}
	o.defaultEventHandler.orchestrator = o
	o.eventHandler = &o.defaultEventHandler

	for _, opt := range opts {
		opt(o)
	}

	return o
}

func (o *Orchestrator) Close() {
	o.closeOnce.Do(func() {
		close(o.closeCh)

		o.conversation.cancelActiveTurn()

		if err := o.stopCapture(); err != nil {
			recordedErr := fmt.Errorf("failed to stop audio capture: %w", err)
			span := trace.SpanFromContext(o.baseContext)
			span.RecordError(recordedErr)
			span.SetStatus(codes.Error, recordedErr.Error())
		}
		if o.audioInput != nil {
			o.audioInput.Close()
		}

		switch c := o.speechToTextClient.(type) {
		case interface{ Close() error }:
			if err := c.Close(); err != nil {
				recordedErr := fmt.Errorf("failed to close speech-to-text client: %w", err)
				span := trace.SpanFromContext(o.baseContext)
				span.RecordError(recordedErr)
				span.SetStatus(codes.Error, recordedErr.Error())
			}
		case interface{ Close() }:
			c.Close()
		}

		if o.assistantLoopStarted.Load() {
			<-o.assistantLoopDone
		}
	})
}

// Orchestrate starts the orchestrator that waits for any triggers to respond to
//
// ctx is used as a base context for any agent and tool calls, allowing for
// cancellation
func (o *Orchestrator) Orchestrate(ctx context.Context, opts ...OrchestrateOption) {
	if o.isClosed() {
		log.Println("Warning: orchestrator already closed, skipping Orchestrate")
		return
	}

	o.orchestrateOptions = OrchestrateOptions{}
	for _, opt := range opts {
		opt(&o.orchestrateOptions)
	}

	o.baseContext = ctx

	o.assistantLoopOnce.Do(func() {
		o.assistantLoopStarted.Store(true)
		go o.startAssistantLoop()
		go func() {
			<-ctx.Done()
			o.Close()
		}()
	})

	if err := o.initSTT(); err != nil {
		recordedErr := fmt.Errorf("failed to initialize speech-to-text: %w", err)
		span := trace.SpanFromContext(o.baseContext)
		span.RecordError(recordedErr)
		span.SetStatus(codes.Error, recordedErr.Error())
	}
	o.initAudioInput()
}

func (o *Orchestrator) SendPrompt(prompt string) {
	o.respondToEvent(events.NewUserPromptEvent(prompt))
}

func (o *Orchestrator) SendAudio(audio []byte) error {
	return o.sendAudio(audio)
}

func (o *Orchestrator) Handle(event llms.EventV0) {
	o.respondToEvent(event)
}

// QueuePrompt immediately queues the prompt for processing after the current
// turn is finished. It bypasses the normal processing pipeline and can be useful
// for handling prompts that are sure to follow up after the current turn.
func (o *Orchestrator) QueuePrompt(prompt string) {
	go o.queueEvent(events.NewUserPromptEvent(prompt))
}

func (o *Orchestrator) isClosed() bool {
	select {
	case <-o.closeCh:
		return true
	default:
		return false
	}
}

func (o *Orchestrator) SetSpeaking(isSpeaking bool) {
	o.IsSpeaking = isSpeaking
	if !isSpeaking {
		o.conversation.stopSpeakingActiveTurn()
	}
	if o.audioOutput != nil {
		o.audioOutput.ClearBuffer()
	}
}

func (o *Orchestrator) IsAlwaysRecording() bool {
	return o.config.AlwaysRecording
}

func (o *Orchestrator) SetAlwaysRecording(isAlwaysRecording bool) {
	o.config.AlwaysRecording = isAlwaysRecording

	if isAlwaysRecording {
		go func() {
			if err := o.startCapture(); err != nil {
				recordedErr := fmt.Errorf("failed to start audio input: %w", err)
				span := trace.SpanFromContext(o.baseContext)
				span.RecordError(recordedErr)
				span.SetStatus(codes.Error, recordedErr.Error())
			}
		}()
	} else if !o.IsRecording {
		if err := o.stopCapture(); err != nil {
			recordedErr := fmt.Errorf("failed to stop audio input: %w", err)
			span := trace.SpanFromContext(o.baseContext)
			span.RecordError(recordedErr)
			span.SetStatus(codes.Error, recordedErr.Error())
		}
	}
}

func (o *Orchestrator) StartRecording() error {
	o.IsRecording = true

	if o.config.AlwaysRecording {
		return nil
	}

	return o.startCapture()
}

func (o *Orchestrator) StopRecording() error {
	o.IsRecording = false
	if o.config.AlwaysRecording {
		return nil
	}

	return o.stopCapture()
}

func (o *Orchestrator) ConversationV0() emaContext.ConversationV0 {
	return &o.conversation
}

func (o *Orchestrator) CallTool(ctx context.Context, prompt string) error {
	ctx, span := tracer.Start(ctx, "call tool with prompt")
	defer span.End()
	history := o.conversation.historySnapshot()

	switch o.llm.(type) {
	case LLMWithStream:
		_, err := o.processStreaming(ctx, events.NewUserPromptEvent(prompt), history, newTextBuffer())
		return err

	case LLMWithPrompt:
		_, err := o.processPromptOld(ctx, events.NewUserPromptEvent(prompt), history, newTextBuffer())
		return err

	default:
		// Impossible state technically
		return fmt.Errorf("unknown LLM type")
	}

}

func (o *Orchestrator) CancelTurn()  { o.conversation.cancelActiveTurn() }
func (o *Orchestrator) PauseTurn()   { o.conversation.pauseActiveTurn() }
func (o *Orchestrator) UnpauseTurn() { o.conversation.unpauseActiveTurn() }

func (o *Orchestrator) initSTT() error {
	if o.speechToTextClient == nil {
		return nil
	}

	sttOptions := []speechtotext.TranscriptionOption{
		speechtotext.WithSpeechStartedCallback(func() {
			go o.respondToEvent(events.NewSpeechStartedEvent())
		}),
		speechtotext.WithSpeechEndedCallback(func() {
			go o.respondToEvent(events.NewSpeechEndedEvent())
		}),
		speechtotext.WithInterimTranscriptionCallback(func(transcript string) {
			go o.respondToEvent(events.NewInterimTranscriptionEvent(transcript))
		}),
		speechtotext.WithTranscriptionCallback(func(transcript string) {
			go o.respondToEvent(events.NewTranscriptionEvent(transcript))
		}),
	}
	if o.audioInput != nil {
		sttOptions = append(sttOptions, speechtotext.WithEncodingInfo(o.audioInput.EncodingInfo()))
	}

	if err := o.speechToTextClient.Transcribe(o.baseContext, sttOptions...); err != nil {
		return fmt.Errorf("failed to start transcribing: %w", err)
	}

	return nil
}
