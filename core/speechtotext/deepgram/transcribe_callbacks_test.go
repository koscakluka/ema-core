package deepgram

import (
	"sync/atomic"
	"testing"

	"github.com/koscakluka/ema-core/core/speechtotext"
)

func TestNewCallbackConfigDefaultsToNoopCallbacks(t *testing.T) {
	callbacks, wsConfig := newCallbackConfig(speechtotext.TranscriptionOptions{})

	callbacks.partialInterimTranscriptionCallback("partial")
	callbacks.interimTranscriptionCallback("interim")
	callbacks.partialTranscriptionCallback("final")
	callbacks.transcriptionCallback("full")
	callbacks.startSpeechCallback()
	callbacks.endSpeechCallback()

	if wsConfig.shouldDetectSpeechStart {
		t.Fatalf("expected speech-start detection disabled when callback is unset")
	}
	if wsConfig.shouldEnhanceSpeechEndingDetection {
		t.Fatalf("expected speech-end enhancement disabled when callbacks are unset")
	}
	if wsConfig.shouldRequestInterimResults {
		t.Fatalf("expected interim-results disabled when callbacks are unset")
	}
}

func TestNewCallbackConfigKeepsConfiguredCallbacksAndFlags(t *testing.T) {
	interimCalls := atomic.Int32{}
	transcriptionCalls := atomic.Int32{}
	startCalls := atomic.Int32{}
	endCalls := atomic.Int32{}
	partialInterimCalls := atomic.Int32{}
	partialFinalCalls := atomic.Int32{}

	callbacks, wsConfig := newCallbackConfig(speechtotext.TranscriptionOptions{
		PartialInterimTranscriptionCallback: func(string) { partialInterimCalls.Add(1) },
		InterimTranscriptionCallback:        func(string) { interimCalls.Add(1) },
		PartialTranscriptionCallback:        func(string) { partialFinalCalls.Add(1) },
		TranscriptionCallback:               func(string) { transcriptionCalls.Add(1) },
		SpeechStartedCallback:               func() { startCalls.Add(1) },
		SpeechEndedCallback:                 func() { endCalls.Add(1) },
	})

	callbacks.partialInterimTranscriptionCallback("hel")
	callbacks.interimTranscriptionCallback("hello")
	callbacks.partialTranscriptionCallback("hello")
	callbacks.transcriptionCallback("hello world")
	callbacks.startSpeechCallback()
	callbacks.endSpeechCallback()

	if !wsConfig.shouldDetectSpeechStart {
		t.Fatalf("expected speech-start detection enabled")
	}
	if !wsConfig.shouldEnhanceSpeechEndingDetection {
		t.Fatalf("expected speech-end enhancement enabled")
	}
	if !wsConfig.shouldRequestInterimResults {
		t.Fatalf("expected interim-results enabled")
	}

	if got := partialInterimCalls.Load(); got != 1 {
		t.Fatalf("expected partial interim callback once, got %d", got)
	}

	if got := interimCalls.Load(); got != 1 {
		t.Fatalf("expected interim callback once, got %d", got)
	}
	if got := partialFinalCalls.Load(); got != 1 {
		t.Fatalf("expected partial transcription callback once, got %d", got)
	}
	if got := transcriptionCalls.Load(); got != 1 {
		t.Fatalf("expected transcription callback once, got %d", got)
	}
	if got := startCalls.Load(); got != 1 {
		t.Fatalf("expected speech-start callback once, got %d", got)
	}
	if got := endCalls.Load(); got != 1 {
		t.Fatalf("expected speech-end callback once, got %d", got)
	}
}
