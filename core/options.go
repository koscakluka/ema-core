package orchestration

import (
	"context"
	"iter"

	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/conversations"
	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/core/speechtotext"
	"github.com/koscakluka/ema-core/core/texttospeech"
)

type OrchestratorOption func(*Orchestrator)

type LLMWithStream interface {
	LLM
	PromptWithStream(ctx context.Context, prompt *string, opts ...llms.StreamingPromptOption) llms.Stream
}

type LLMWithGeneralPrompt interface {
	LLM
	Prompt(ctx context.Context, prompt string, opts ...llms.GeneralPromptOption) (*llms.Message, error)
}

func WithStreamingLLM(client LLMWithStream) OrchestratorOption {
	return func(o *Orchestrator) {
		o.llm.set(client)
	}
}

type SpeechToText interface {
	Transcribe(ctx context.Context, opts ...speechtotext.TranscriptionOption) error
	SendAudio(audio []byte) error
}

func WithSpeechToTextClient(client SpeechToText) OrchestratorOption {
	return func(o *Orchestrator) {
		o.speechToText.set(client)
	}
}

type TextToSpeech interface {
	OpenStream(ctx context.Context, opts ...texttospeech.TextToSpeechOption) error
	SendText(text string) error
	FlushBuffer() error
}

func WithTextToSpeechClient(client TextToSpeech) OrchestratorOption {
	return func(o *Orchestrator) {
		o.textToSpeech.set(client)
		o.IsSpeaking = true
		// TODO: This doesn't really make sense here
		o.textToSpeech.Unmute()
	}
}

type TextToSpeechV1 interface {
	NewSpeechGeneratorV0(ctx context.Context, opts ...texttospeech.TextToSpeechOption) (texttospeech.SpeechGeneratorV0, error)
}

func WithTextToSpeechClientV1(client TextToSpeechV1) OrchestratorOption {
	return func(o *Orchestrator) {
		o.textToSpeech.set(client)
		o.IsSpeaking = true
		// TODO: This doesn't really make sense here
		o.textToSpeech.Unmute()
	}
}

type AudioInput interface {
	audioInputBase
}

type AudioInputFine interface {
	StartCapture(ctx context.Context, onAudio func(audio []byte)) error
	StopCapture() error
}

func WithAudioInput(client AudioInput) OrchestratorOption {
	return func(o *Orchestrator) { o.audioInput.Set(client) }
}

type AudioOutputV0 interface {
	audioOutputBase
	AwaitMark() error
}

func WithAudioOutputV0(client AudioOutputV0) OrchestratorOption {
	return func(o *Orchestrator) { o.audioOutput.Set(client) }
}

type AudioOutputV1 interface {
	audioOutputBase
	Mark(string, func(string)) error
}

func WithAudioOutputV1(client AudioOutputV1) OrchestratorOption {
	return func(o *Orchestrator) { o.audioOutput.Set(client) }
}

func WithTools(tools ...llms.Tool) OrchestratorOption {
	return func(o *Orchestrator) { o.llm.setTools(tools...) }
}

func WithOrchestrationTools() OrchestratorOption {
	return func(o *Orchestrator) { o.llm.appendTools(orchestrationTools(o)...) }
}

type TriggerHandlerV0 interface {
	HandleTriggerV0(ctx context.Context, trigger llms.TriggerV0, conversation conversations.ActiveContextV0) iter.Seq2[llms.TriggerV0, error]
}

func WithTriggerHandlerV0(handler TriggerHandlerV0) OrchestratorOption {
	return func(o *Orchestrator) {
		if handler == nil {
			o.triggerHandler = &o.defaultTriggerHandler
			return
		}
		o.triggerHandler = handler
	}
}

type OrchestrateOptions struct {
	onTranscription               func(transcript string)
	onPartialTranscription        func(transcript string)
	onInterimTranscription        func(transcript string)
	onPartialInterimTranscription func(transcript string)
	onSpeakingStateChanged        func(isSpeaking bool)
	onResponse                    func(response string)
	onResponseEnd                 func()
	onCancellation                func()
	onInputAudio                  func(audio []byte)
	onAudio                       func(audio []byte)
	onAudioEnded                  func(transcript string)
	onSpokenText                  func(spokenText string)
	onSpokenTextDelta             func(spokenTextDelta string)
}

type OrchestrateOption func(*OrchestrateOptions)

// WithTranscriptionCallback registers a callback for final transcriptions
// produced by the configured speech-to-text client.
//
// Triggers manually submitted through [Orchestrator.HandleTrigger] do not trigger this
// callback.
func WithTranscriptionCallback(callback func(transcript string)) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onTranscription = callback
	}
}

// WithPartialTranscriptionCallback registers a callback for finalized
// transcription segments produced by the configured speech-to-text client.
//
// Triggers manually submitted through [Orchestrator.HandleTrigger] do not trigger this
// callback.
func WithPartialTranscriptionCallback(callback func(transcript string)) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onPartialTranscription = callback
	}
}

// WithInterimTranscriptionCallback registers a callback for interim
// transcriptions produced by the configured speech-to-text client.
//
// Triggers manually submitted through [Orchestrator.HandleTrigger] do not trigger this
// callback.
func WithInterimTranscriptionCallback(callback func(transcript string)) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onInterimTranscription = callback
	}
}

// WithPartialInterimTranscriptionCallback registers a callback for partial
// interim transcriptions produced by the configured speech-to-text client.
//
// Triggers manually submitted through [Orchestrator.HandleTrigger] do not trigger this
// callback.
func WithPartialInterimTranscriptionCallback(callback func(transcript string)) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onPartialInterimTranscription = callback
	}
}

// WithSpeakingStateChangedCallback registers a callback for speaking-state
// updates produced by the configured speech-to-text client.
//
// Triggers manually submitted through [Orchestrator.HandleTrigger] do not trigger this
// callback.
func WithSpeakingStateChangedCallback(callback func(isSpeaking bool)) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onSpeakingStateChanged = callback
	}
}

func WithResponseCallback(callback func(response string)) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onResponse = callback
	}
}

func WithResponseEndCallback(callback func()) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onResponseEnd = callback
	}
}

func WithCancellationCallback(callback func()) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onCancellation = callback
	}
}

func WithAudioCallback(callback func(audio []byte)) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onAudio = callback
	}
}

func WithAudioEndedCallback(callback func(transcript string)) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onAudioEnded = callback
	}
}

// WithSpokenTextCallback registers a callback for spoken-text progress updates.
//
// The callback receives mark-confirmed text plus a best-effort approximation of
// the current in-flight segment while audio is playing.
func WithSpokenTextCallback(callback func(spokenText string)) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onSpokenText = callback
	}
}

// WithSpokenTextDeltaCallback registers a callback for spoken-text deltas.
//
// The callback receives append-only incremental transcript segments. If
// playback progress regresses, no replacement segment is emitted.
func WithSpokenTextDeltaCallback(callback func(spokenTextDelta string)) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onSpokenTextDelta = callback
	}
}

// WithInputAudioCallback registers a callback for raw input audio chunks.
//
// The provided slice is passed through as-is (no defensive copy). Receivers
// can choose whether to process it immediately, copy it, or retain it. The
// callback runs inline on the input-audio path and should not block.
func WithInputAudioCallback(callback func(audio []byte)) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onInputAudio = callback
	}
}

type LLM any

type audioOutputBase interface {
	EncodingInfo() audio.EncodingInfo
	SendAudio(audio []byte) error
	ClearBuffer()
}

type audioInputBase interface {
	EncodingInfo() audio.EncodingInfo
	Stream(ctx context.Context, onAudio func(audio []byte)) error
	Close()
}

type textToSpeechBase any
