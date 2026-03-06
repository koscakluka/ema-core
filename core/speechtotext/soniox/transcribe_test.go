package soniox

import (
	"sync/atomic"
	"testing"

	"github.com/koscakluka/ema-core/core/speechtotext"
)

func TestNewCallbackConfigDefaultsToNoopCallbacks(t *testing.T) {
	callbacks := newCallbackConfig(speechtotext.TranscriptionOptions{})

	callbacks.partialInterimTranscriptionCallback("partial")
	callbacks.interimTranscriptionCallback("interim")
	callbacks.partialTranscriptionCallback("final")
	callbacks.transcriptionCallback("full")
	callbacks.startSpeechCallback()
	callbacks.endSpeechCallback()
}

func TestProcessResponseEmitsSpeechLifecycleAndTranscript(t *testing.T) {
	client := &TranscriptionClient{}

	startCalls := atomic.Int32{}
	endCalls := atomic.Int32{}
	partialInterim := ""
	interim := ""
	partialFinal := ""
	finalTranscript := ""

	callbacks := callbackConfig{
		partialInterimTranscriptionCallback: func(transcript string) { partialInterim = transcript },
		interimTranscriptionCallback:        func(transcript string) { interim = transcript },
		partialTranscriptionCallback:        func(transcript string) { partialFinal = transcript },
		transcriptionCallback:               func(transcript string) { finalTranscript = transcript },
		startSpeechCallback:                 func() { startCalls.Add(1) },
		endSpeechCallback:                   func() { endCalls.Add(1) },
	}

	client.processResponse(responseMessage{
		Tokens: []responseToken{{Text: "hel", IsFinal: false}},
	}, callbacks)

	if got := startCalls.Load(); got != 1 {
		t.Fatalf("expected one speech-start callback, got %d", got)
	}
	if partialInterim != "hel" {
		t.Fatalf("expected interim segment hel, got %q", partialInterim)
	}
	if interim != "hel" {
		t.Fatalf("expected interim transcript hel, got %q", interim)
	}

	client.processResponse(responseMessage{
		Tokens: []responseToken{
			{Text: "hello", IsFinal: true},
			{Text: "<end>", IsFinal: true},
		},
	}, callbacks)

	if partialFinal != "hello" {
		t.Fatalf("expected partial final transcript hello, got %q", partialFinal)
	}
	if finalTranscript != "hello" {
		t.Fatalf("expected final transcript hello, got %q", finalTranscript)
	}
	if got := endCalls.Load(); got != 1 {
		t.Fatalf("expected one speech-end callback, got %d", got)
	}
}
