package orchestration

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/core/speechtotext"
)

func TestSpeechToTextStartForwardsCallbacksAndInvokesEvents(t *testing.T) {
	sttClient := &speechToTextClientStub{
		transcribe: func(opts speechtotext.TranscriptionOptions) {
			if opts.SpeechStartedCallback == nil {
				t.Fatalf("expected speech-start callback to be configured")
			}
			if opts.SpeechEndedCallback == nil {
				t.Fatalf("expected speech-end callback to be configured")
			}
			if opts.PartialInterimTranscriptionCallback == nil {
				t.Fatalf("expected partial interim callback to be configured")
			}
			if opts.InterimTranscriptionCallback == nil {
				t.Fatalf("expected interim callback to be configured")
			}
			if opts.PartialTranscriptionCallback == nil {
				t.Fatalf("expected partial transcription callback to be configured")
			}
			if opts.TranscriptionCallback == nil {
				t.Fatalf("expected transcription callback to be configured")
			}

			opts.SpeechStartedCallback()
			opts.PartialInterimTranscriptionCallback("hel")
			opts.InterimTranscriptionCallback("hel")
			opts.PartialTranscriptionCallback("hello")
			opts.TranscriptionCallback("hello")
			opts.SpeechEndedCallback()
		},
	}

	runtime := newSpeechToText(sttClient)

	var mu sync.Mutex
	states := []bool{}
	partialInterim := []string{}
	interim := []string{}
	partialTranscriptions := []string{}
	transcriptions := []string{}
	invokedEvents := atomic.Int32{}
	invokedEventNames := []string{}

	runtime.SetSpeechStateChangedCallback(func(isSpeaking bool) {
		mu.Lock()
		states = append(states, isSpeaking)
		mu.Unlock()
	})
	runtime.SetInterimTranscriptionCallback(func(transcript string) {
		mu.Lock()
		interim = append(interim, transcript)
		mu.Unlock()
	})
	runtime.SetPartialInterimTranscriptionCallback(func(transcript string) {
		mu.Lock()
		partialInterim = append(partialInterim, transcript)
		mu.Unlock()
	})
	runtime.SetPartialTranscriptionCallback(func(transcript string) {
		mu.Lock()
		partialTranscriptions = append(partialTranscriptions, transcript)
		mu.Unlock()
	})
	runtime.SetTranscriptionCallback(func(transcript string) {
		mu.Lock()
		transcriptions = append(transcriptions, transcript)
		mu.Unlock()
	})
	runtime.SetInvokeTrigger(func(trigger llms.TriggerV0) {
		if trigger != nil {
			invokedEvents.Add(1)
			mu.Lock()
			invokedEventNames = append(invokedEventNames, trigger.String())
			mu.Unlock()
		}
	})

	encodingInfo := audio.GetDefaultEncodingInfo()
	if err := runtime.Start(context.Background(), &encodingInfo); err != nil {
		t.Fatalf("expected start to succeed, got %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for invokedEvents.Load() < 4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(states) != 2 || !states[0] || states[1] {
		t.Fatalf("expected speaking states [true false], got %v", states)
	}

	if len(interim) != 2 || interim[0] != "hel" || interim[1] != "" {
		t.Fatalf("expected interim callbacks [\"hel\" \"\"], got %v", interim)
	}

	if len(partialInterim) != 2 || partialInterim[0] != "hel" || partialInterim[1] != "" {
		t.Fatalf("expected partial interim callbacks [\"hel\" \"\"], got %v", partialInterim)
	}

	if len(partialTranscriptions) != 1 || partialTranscriptions[0] != "hello" {
		t.Fatalf("expected partial transcription callback [\"hello\"], got %v", partialTranscriptions)
	}

	if len(transcriptions) != 1 || transcriptions[0] != "hello" {
		t.Fatalf("expected transcription callback [\"hello\"], got %v", transcriptions)
	}

	if got := invokedEvents.Load(); got != 4 {
		t.Fatalf("expected 4 invoked events, got %d (%v)", got, invokedEventNames)
	}
}

func TestSpeechToTextIndividualSettersAcceptNilAndClearCallbacks(t *testing.T) {
	runtime := newSpeechToText(nil)
	invocations := atomic.Int32{}

	runtime.SetSpeechStateChangedCallback(func(bool) { invocations.Add(1) })
	runtime.SetInterimTranscriptionCallback(func(string) { invocations.Add(1) })
	runtime.SetPartialInterimTranscriptionCallback(func(string) { invocations.Add(1) })
	runtime.SetPartialTranscriptionCallback(func(string) { invocations.Add(1) })
	runtime.SetTranscriptionCallback(func(string) { invocations.Add(1) })
	runtime.SetInvokeTrigger(func(llms.TriggerV0) { invocations.Add(1) })

	runtime.SetSpeechStateChangedCallback(nil)
	runtime.SetInterimTranscriptionCallback(nil)
	runtime.SetPartialInterimTranscriptionCallback(nil)
	runtime.SetPartialTranscriptionCallback(nil)
	runtime.SetTranscriptionCallback(nil)
	runtime.SetInvokeTrigger(nil)

	runtime.invokeSpeechStarted()
	runtime.invokeSpeechEnded()
	runtime.invokePartialInterimTranscription("partial")
	runtime.invokeInterimTranscription("partial")
	runtime.invokePartialTranscription("partial final")
	runtime.invokeTranscription("final")

	if got := invocations.Load(); got != 0 {
		t.Fatalf("expected callbacks to be cleared, got %d invocations", got)
	}
}

type speechToTextClientStub struct {
	transcribe func(opts speechtotext.TranscriptionOptions)
}

func (stub *speechToTextClientStub) Transcribe(_ context.Context, opts ...speechtotext.TranscriptionOption) error {
	transcriptionOptions := speechtotext.TranscriptionOptions{}
	for _, opt := range opts {
		opt(&transcriptionOptions)
	}

	if stub.transcribe != nil {
		stub.transcribe(transcriptionOptions)
	}

	return nil
}

func (stub *speechToTextClientStub) SendAudio([]byte) error {
	return nil
}
