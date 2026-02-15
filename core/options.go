package orchestration

import (
	"context"

	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/interruptions"
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
		o.llm = client
	}
}

type SpeechToText interface {
	Transcribe(ctx context.Context, opts ...speechtotext.TranscriptionOption) error
	SendAudio(audio []byte) error
}

func WithSpeechToTextClient(client SpeechToText) OrchestratorOption {
	return func(o *Orchestrator) {
		o.speechToTextClient = client
	}
}

type TextToSpeech interface {
	OpenStream(ctx context.Context, opts ...texttospeech.TextToSpeechOption) error
	SendText(text string) error
	FlushBuffer() error
}

func WithTextToSpeechClient(client TextToSpeech) OrchestratorOption {
	return func(o *Orchestrator) {
		o.textToSpeechClient = client
		o.IsSpeaking = true
	}
}

type TextToSpeechV1 interface {
	NewSpeechGeneratorV0(ctx context.Context, opts ...texttospeech.TextToSpeechOption) (texttospeech.SpeechGeneratorV0, error)
}

func WithTextToSpeechClientV1(client TextToSpeechV1) OrchestratorOption {
	return func(o *Orchestrator) {
		o.textToSpeechClient = client
		o.IsSpeaking = true
	}
}

type AudioInput interface {
	EncodingInfo() audio.EncodingInfo
	Stream(ctx context.Context, onAudio func(audio []byte)) error
	Close()
}

type AudioInputFine interface {
	StartCapture(ctx context.Context, onAudio func(audio []byte)) error
	StopCapture() error
}

func WithAudioInput(client AudioInput) OrchestratorOption {
	return func(o *Orchestrator) {
		o.audioInput = client
	}
}

type AudioOutputV0 interface {
	audioOutput
	AwaitMark() error
}

func WithAudioOutputV0(client AudioOutputV0) OrchestratorOption {
	return func(o *Orchestrator) {
		o.audioOutput = client
	}
}

type AudioOutputV1 interface {
	audioOutput
	Mark(string, func(string)) error
}

func WithAudioOutputV1(client AudioOutputV1) OrchestratorOption {
	return func(o *Orchestrator) {
		o.audioOutput = client
	}
}

func WithTools(tools ...llms.Tool) OrchestratorOption {
	return func(o *Orchestrator) {
		o.tools = tools
	}
}

func WithOrchestrationTools() OrchestratorOption {
	return func(o *Orchestrator) {
		o.tools = append(o.tools, orchestrationTools(o)...)
	}
}

type InterruptionHandlerV0 interface {
	HandleV0(prompt string, turns []llms.Turn, tools []llms.Tool, orchestrator interruptions.OrchestratorV0) error
}

func WithInterruptionHandlerV0(handler InterruptionHandlerV0) OrchestratorOption {
	return func(o *Orchestrator) {
		o.interruptionHandlerV0 = handler
	}
}

type InterruptionHandlerV1 interface {
	HandleV1(id int64, orchestrator interruptions.OrchestratorV0, tools []llms.Tool) (*llms.InterruptionV0, error)
}

func WithInterruptionHandlerV1(handler InterruptionHandlerV1) OrchestratorOption {
	return func(o *Orchestrator) {
		o.interruptionHandlerV1 = handler
	}
}

type InterruptionHandlerV2 interface {
	HandleV2(ctx context.Context, id int64, orchestrator interruptions.OrchestratorV0, tools []llms.Tool) (*llms.InterruptionV0, error)
}

func WithInterruptionHandlerV2(handler InterruptionHandlerV2) OrchestratorOption {
	return func(o *Orchestrator) {
		o.interruptionHandlerV2 = handler
	}
}

func WithConfig(config *Config) OrchestratorOption {
	return func(o *Orchestrator) {
		if config == nil {
			return
		}

		o.config = config
	}
}

type OrchestrateOptions struct {
	onTranscription        func(transcript string)
	onInterimTranscription func(transcript string)
	onSpeakingStateChanged func(isSpeaking bool)
	onResponse             func(response string)
	onResponseEnd          func()
	onCancellation         func()
	onInputAudio           func(audio []byte)
	onAudio                func(audio []byte)
	onAudioEnded           func(transcript string)
}

type OrchestrateOption func(*OrchestrateOptions)

func WithTranscriptionCallback(callback func(transcript string)) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onTranscription = callback
	}
}

func WithInterimTranscriptionCallback(callback func(transcript string)) OrchestrateOption {
	return func(o *OrchestrateOptions) {
		o.onInterimTranscription = callback
	}
}

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

type audioOutput interface {
	EncodingInfo() audio.EncodingInfo
	SendAudio(audio []byte) error
	ClearBuffer()
}

type textToSpeech any
