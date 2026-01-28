package orchestration

import (
	"context"
	"fmt"
	"sync"

	"log"

	emaContext "github.com/koscakluka/ema-core/core/context"
	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/trace"
)

type Orchestrator struct {
	IsRecording bool
	IsSpeaking  bool

	turns Turns

	outputTextBuffer  textBuffer
	outputAudioBuffer audioBuffer
	transcripts       chan promptQueueItem
	promptEnded       sync.WaitGroup

	tools []llms.Tool

	llm                    LLM
	speechToTextClient     SpeechToText
	textToSpeechClient     TextToSpeech
	audioInput             AudioInput
	audioOutput            audioOutput
	interruptionClassifier InterruptionClassifier
	interruptionHandlerV0  InterruptionHandlerV0
	interruptionHandlerV1  InterruptionHandlerV1
	interruptionHandlerV2  InterruptionHandlerV2

	orchestrateOptions OrchestrateOptions
	config             *Config

	baseContext context.Context
}

func NewOrchestrator(opts ...OrchestratorOption) *Orchestrator {
	o := &Orchestrator{
		IsRecording:       false,
		IsSpeaking:        false,
		transcripts:       make(chan promptQueueItem, 10), // TODO: Figure out good valiues for this
		config:            &Config{AlwaysRecording: true},
		turns:             Turns{activeTurnIdx: -1},
		outputTextBuffer:  *newTextBuffer(),
		outputAudioBuffer: *newAudioBuffer(),
		baseContext:       context.Background(),
	}

	for _, opt := range opts {
		opt(o)
	}

	// TODO: Remove this in a couple of releases
	if o.interruptionClassifier == nil {
		switch o.llm.(type) {
		case LLMWithPrompt:
			o.interruptionClassifier = NewSimpleInterruptionClassifier(o.llm.(LLMWithPrompt))
		case InterruptionLLM:
			// HACK: To avoid changing the signature of
			// NewSimpleInterruptionClassifier we pass nil for LLM right now,
			// when we change the whole classifier concept we can change the
			// signature
			o.interruptionClassifier = NewSimpleInterruptionClassifier(nil, ClassifierWithInterruptionLLM(o.llm.(InterruptionLLM)))
		case LLMWithGeneralPrompt:
			// HACK: To avoid changing the signature of
			// NewSimpleInterruptionClassifier we pass nil for LLM right now,
			// when we change the whole classifier concept we can change the
			// signature
			o.interruptionClassifier = NewSimpleInterruptionClassifier(nil, ClassifierWithGeneralPromptLLM(o.llm.(LLMWithGeneralPrompt)))
		}
	}

	return o
}

func (o *Orchestrator) Close() {
	// TODO: Make sure that deepgramClient is closed and no longer transcribing
	// before closing the channel
	close(o.transcripts)
	trace.SpanFromContext(o.turns.activeTurnCtx).End()
}

// Orchestrate starts the orchestrator that waits for any triggers to respond to
//
// ctx is used as a base context for any agent and tool calls, allowing for
// cancellation
func (o *Orchestrator) Orchestrate(ctx context.Context, opts ...OrchestrateOption) {
	o.orchestrateOptions = OrchestrateOptions{}
	for _, opt := range opts {
		opt(&o.orchestrateOptions)
	}

	o.baseContext = ctx

	o.initTTS()
	o.initSST()

	go o.startAssistantLoop()
	o.initAudioInput()
}

func (o *Orchestrator) SendPrompt(prompt string) {
	o.processUserTurn(prompt)
}

func (o *Orchestrator) SendAudio(audio []byte) error {
	return o.sendAudio(audio)
}

// QueuePrompt immediately queues the prompt for processing after the current
// turn is finished. It bypasses the normal processing pipeline and can be useful
// for handling prompts that are sure to follow up after the current turn.
func (o *Orchestrator) QueuePrompt(prompt string) {
	go o.queuePrompt(prompt)
}

func (o *Orchestrator) SetSpeaking(isSpeaking bool) {
	o.IsSpeaking = isSpeaking
	o.outputAudioBuffer.AddAudio([]byte{})
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
				log.Printf("Failed to start audio input: %v", err)
			}
		}()
	} else if !o.IsRecording {
		if err := o.stopCapture(); err != nil {
			log.Printf("Failed to stop audio input: %v", err)
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

func (o *Orchestrator) Turns() emaContext.TurnsV0 {
	return &o.turns
}

func (o *Orchestrator) CallTool(ctx context.Context, prompt string) error {
	ctx, span := tracer.Start(ctx, "call tool with prompt")
	defer span.End()
	switch o.llm.(type) {
	case LLMWithStream:
		_, err := o.processStreaming(ctx, prompt, o.turns.turns, newTextBuffer())
		return err

	case LLMWithPrompt:
		_, err := o.processPromptOld(ctx, prompt, o.turns.turns, newTextBuffer())
		return err

	default:
		// Impossible state technically
		return fmt.Errorf("unknown LLM type")
	}

}
