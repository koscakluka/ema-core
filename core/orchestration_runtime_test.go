package orchestration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/llms"
)

func TestCloseBeforeOrchestrateMarksClosed(t *testing.T) {
	o := NewOrchestrator()
	o.Close()

	if !o.runtime.isClosed() {
		t.Fatalf("expected orchestrator to be closed")
	}

	o.Orchestrate(context.Background())
	if !o.runtime.isClosed() {
		t.Fatalf("expected orchestrator to stay closed")
	}
}

func TestQueuePromptBeforeOrchestrateIsProcessed(t *testing.T) {
	o := NewOrchestrator(WithLLM(promptLLMStub{response: "queued response"}))
	defer o.Close()

	o.QueuePrompt("queued prompt")

	responseReceived := make(chan string, 1)
	responseEnded := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o.Orchestrate(ctx,
		WithResponseCallback(func(response string) {
			select {
			case responseReceived <- response:
			default:
			}
		}),
		WithResponseEndCallback(func() {
			select {
			case responseEnded <- struct{}{}:
			default:
			}
		}),
	)

	select {
	case <-responseEnded:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for queued prompt to finish")
	}

	select {
	case response := <-responseReceived:
		if response != "queued response" {
			t.Fatalf("expected queued response, got %q", response)
		}
	default:
		t.Fatalf("expected a queued response callback")
	}
}

func TestCancelTurnCancelsActiveTurn(t *testing.T) {
	o := NewOrchestrator(WithStreamingLLM(repeatingStreamLLMStub{chunk: "chunk", interval: 10 * time.Millisecond}))
	defer o.Close()

	responseReceived := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o.Orchestrate(ctx,
		WithResponseCallback(func(string) {
			select {
			case responseReceived <- struct{}{}:
			default:
			}
		}),
		WithCancellationCallback(func() {
			select {
			case cancelled <- struct{}{}:
			default:
			}
		}),
	)

	o.SendPrompt("please start")

	select {
	case <-responseReceived:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for active turn to start")
	}

	o.CancelTurn()

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for cancellation callback")
	}
}

func TestSetSpeakingTrueDoesNotStopActiveTurn(t *testing.T) {
	audioOutput := &recordingAudioOutput{}
	o := NewOrchestrator(
		WithStreamingLLM(repeatingStreamLLMStub{chunk: "chunk", interval: 10 * time.Millisecond}),
		WithAudioOutputV0(audioOutput),
	)
	defer o.Close()

	o.SetSpeaking(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o.Orchestrate(ctx)
	o.SendPrompt("keep speaking")

	waitForCondition(t, 2*time.Second, "active turn to start", func() bool {
		o.conversation.mu.RLock()
		defer o.conversation.mu.RUnlock()
		return o.conversation.activeTurn != nil
	})

	o.SetSpeaking(true)

	if clearCalls := audioOutput.clearCalls(); clearCalls != 0 {
		t.Fatalf("expected no audio output clear calls when enabling speaking, got %d", clearCalls)
	}

	o.CancelTurn()
}

func TestCallToolWithoutLLMReturnsExplicitError(t *testing.T) {
	o := NewOrchestrator()

	err := o.CallTool(context.Background(), "test")
	if !errors.Is(err, ErrLLMNotConfigured) {
		t.Fatalf("expected ErrLLMNotConfigured, got %v", err)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, description string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", description)
}

type promptLLMStub struct {
	response string
}

func (stub promptLLMStub) Prompt(_ context.Context, _ string, opts ...llms.PromptOption) ([]llms.Message, error) {
	promptOptions := llms.PromptOptions{}
	for _, opt := range opts {
		opt(&promptOptions)
	}

	if promptOptions.Stream != nil {
		promptOptions.Stream(stub.response)
	}

	response := llms.Message{Content: stub.response}
	return []llms.Message{response}, nil
}

type repeatingStreamLLMStub struct {
	chunk    string
	interval time.Duration
}

func (stub repeatingStreamLLMStub) PromptWithStream(context.Context, *string, ...llms.StreamingPromptOption) llms.Stream {
	return repeatingStreamStub{
		chunk:    stub.chunk,
		interval: stub.interval,
	}
}

type repeatingStreamStub struct {
	chunk    string
	interval time.Duration
}

func (stub repeatingStreamStub) Chunks(ctx context.Context) func(func(llms.StreamChunk, error) bool) {
	return func(yield func(llms.StreamChunk, error) bool) {
		ticker := time.NewTicker(stub.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !yield(streamContentChunkStub{content: stub.chunk}, nil) {
					return
				}
			}
		}
	}
}

type streamContentChunkStub struct {
	content string
}

func (chunk streamContentChunkStub) FinishReason() *string {
	return nil
}

func (chunk streamContentChunkStub) Content() string {
	return chunk.content
}

type recordingAudioOutput struct {
	mu         sync.Mutex
	clearCount int
}

func (output *recordingAudioOutput) EncodingInfo() audio.EncodingInfo {
	return audio.GetDefaultEncodingInfo()
}

func (output *recordingAudioOutput) SendAudio([]byte) error {
	return nil
}

func (output *recordingAudioOutput) ClearBuffer() {
	output.mu.Lock()
	output.clearCount++
	output.mu.Unlock()
}

func (output *recordingAudioOutput) AwaitMark() error {
	return nil
}

func (output *recordingAudioOutput) clearCalls() int {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.clearCount
}
