package orchestration

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/events"
)

func TestResponseEndCallbackFiresWithoutLLM(t *testing.T) {
	o := NewOrchestrator()
	defer o.Close()

	responseEnded := make(chan struct{}, 1)
	responseEndCalls := atomic.Int32{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o.Orchestrate(ctx, WithResponseEndCallback(func() {
		if responseEndCalls.Add(1) == 1 {
			select {
			case responseEnded <- struct{}{}:
			default:
			}
		}
	}))

	o.SendPrompt("no llm configured")

	select {
	case <-responseEnded:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for response end callback")
	}

	time.Sleep(50 * time.Millisecond)
	if got := responseEndCalls.Load(); got != 1 {
		t.Fatalf("expected one response end callback, got %d", got)
	}
}

func TestCallToolWithoutLLMIsNoop(t *testing.T) {
	o := NewOrchestrator()

	if err := o.CallTool(context.Background(), "no configured llm"); err != nil {
		t.Fatalf("expected no error when no llm is configured, got %v", err)
	}
}

func TestHandleTranscriptionEventDoesNotTriggerTranscriptionCallback(t *testing.T) {
	o := NewOrchestrator()
	defer o.Close()

	transcriptionCalls := atomic.Int32{}
	responseEnded := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o.Orchestrate(ctx,
		WithTranscriptionCallback(func(string) {
			transcriptionCalls.Add(1)
		}),
		WithResponseEndCallback(func() {
			select {
			case responseEnded <- struct{}{}:
			default:
			}
		}),
	)

	o.Handle(events.NewTranscriptionEvent("manual transcription event"))

	select {
	case <-responseEnded:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for manual event turn processing")
	}

	time.Sleep(50 * time.Millisecond)
	if got := transcriptionCalls.Load(); got != 0 {
		t.Fatalf("expected manual transcription event to skip callback, got %d callback calls", got)
	}
}

func TestWithInputAudioCallbackReceivesInputAudio(t *testing.T) {
	expected := [][]byte{{0x01, 0x02}, {0x03, 0x04}}

	o := NewOrchestrator(WithAudioInput(&scriptedAudioInputClient{chunks: expected}))
	defer o.Close()

	received := make(chan []byte, len(expected))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o.Orchestrate(ctx, WithInputAudioCallback(func(audio []byte) {
		copied := append([]byte(nil), audio...)
		select {
		case received <- copied:
		default:
		}
	}))

	for i, want := range expected {
		select {
		case got := <-received:
			if !bytes.Equal(got, want) {
				t.Fatalf("expected audio chunk %d to be %v, got %v", i, want, got)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for audio chunk %d", i)
		}
	}
}

type scriptedAudioInputClient struct {
	chunks [][]byte
}

func (s *scriptedAudioInputClient) EncodingInfo() audio.EncodingInfo {
	return audio.GetDefaultEncodingInfo()
}

func (s *scriptedAudioInputClient) Stream(ctx context.Context, onAudio func(audio []byte)) error {
	for _, chunk := range s.chunks {
		select {
		case <-ctx.Done():
			return nil
		default:
			onAudio(chunk)
		}
	}

	return nil
}

func (s *scriptedAudioInputClient) Close() {}
