package orchestration

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/conversations"
	"github.com/koscakluka/ema-core/core/llms"
)

func TestCloseBeforeOrchestrateMarksClosed(t *testing.T) {
	o := NewOrchestrator()
	o.Close()

	if o.eventPlayer.CanIngest() {
		t.Fatalf("expected orchestrator to be closed")
	}

	o.Orchestrate(context.Background())
	if o.eventPlayer.CanIngest() {
		t.Fatalf("expected orchestrator to stay closed")
	}
}

func TestControlOperationsWithoutActivePipelineDoNotPanic(t *testing.T) {
	o := NewOrchestrator()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected no panic when no active pipeline, got %v", recovered)
		}
	}()

	o.CancelTurn()
	o.PauseTurn()
	o.UnpauseTurn()
	o.Mute()
	o.currentResponsePipeline().Close()

	if !o.eventPlayer.CanIngest() {
		t.Fatalf("expected orchestrator to remain open after no-op control operations")
	}
}

func TestCurrentActiveContextPrefersPipelineThenBaseThenBackground(t *testing.T) {
	type contextKey string
	const sourceKey contextKey = "source"

	pipelineCtx := context.WithValue(context.Background(), sourceKey, "pipeline")
	baseCtx := context.WithValue(context.Background(), sourceKey, "base")

	o := NewOrchestrator()
	o.baseContext = baseCtx
	o.responsePipeline.Store(&responsePipeline{ctx: pipelineCtx})

	if got := o.currentActiveContext().Value(sourceKey); got != "pipeline" {
		t.Fatalf("expected active pipeline context to take precedence, got %v", got)
	}

	o.responsePipeline.Store(nil)
	if got := o.currentActiveContext().Value(sourceKey); got != "base" {
		t.Fatalf("expected base context fallback, got %v", got)
	}

	o.baseContext = nil
	if got := o.currentActiveContext(); got == nil {
		t.Fatalf("expected background context fallback, got nil")
	}

	var nilOrchestrator *Orchestrator
	if got := nilOrchestrator.currentActiveContext(); got == nil {
		t.Fatalf("expected nil orchestrator to return background context, got nil")
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

func TestMidTurnLLMSwapAffectsOnlyNextTurn(t *testing.T) {
	firstTurnLLM := scriptedStreamLLMStub{chunks: []string{"A1", "A2"}, interval: 10 * time.Millisecond}
	secondTurnLLM := scriptedStreamLLMStub{chunks: []string{"B1", "B2"}, interval: 10 * time.Millisecond}

	o := NewOrchestrator(WithStreamingLLM(firstTurnLLM))
	defer o.Close()

	firstTurnChunks := []string{}
	secondTurnChunks := []string{}
	chunksMu := sync.Mutex{}
	currentTurn := 1

	firstChunkSeen := make(chan struct{}, 1)
	firstTurnEnded := make(chan struct{}, 1)
	secondTurnEnded := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o.Orchestrate(ctx,
		WithResponseCallback(func(response string) {
			chunksMu.Lock()
			if currentTurn == 1 {
				firstTurnChunks = append(firstTurnChunks, response)
				if len(firstTurnChunks) == 1 {
					select {
					case firstChunkSeen <- struct{}{}:
					default:
					}
				}
			} else {
				secondTurnChunks = append(secondTurnChunks, response)
			}
			chunksMu.Unlock()
		}),
		WithResponseEndCallback(func() {
			chunksMu.Lock()
			if currentTurn == 1 {
				currentTurn = 2
				select {
				case firstTurnEnded <- struct{}{}:
				default:
				}
			} else {
				select {
				case secondTurnEnded <- struct{}{}:
				default:
				}
			}
			chunksMu.Unlock()
		}),
	)

	o.SendPrompt("first turn")

	select {
	case <-firstChunkSeen:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for first turn chunk")
	}

	WithStreamingLLM(secondTurnLLM)(o)

	select {
	case <-firstTurnEnded:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for first turn completion")
	}

	waitForCondition(t, 2*time.Second, "first turn cleanup", func() bool {
		o.conversation.mu.RLock()
		defer o.conversation.mu.RUnlock()
		return o.conversation.activeTurn == nil
	})

	o.SendPrompt("second turn")

	select {
	case <-secondTurnEnded:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for second turn completion")
	}

	if len(firstTurnChunks) == 0 {
		t.Fatalf("expected first turn chunks")
	}
	if len(secondTurnChunks) == 0 {
		t.Fatalf("expected second turn chunks")
	}

	for _, chunk := range firstTurnChunks {
		if chunk != "A1" && chunk != "A2" {
			t.Fatalf("expected first turn to keep initial LLM chunks, got %q", chunk)
		}
	}
	for _, chunk := range secondTurnChunks {
		if chunk != "B1" && chunk != "B2" {
			t.Fatalf("expected second turn to use updated LLM chunks, got %q", chunk)
		}
	}
}

func TestCancelTurnEmitsCancellationCallbackOnce(t *testing.T) {
	o := NewOrchestrator(WithStreamingLLM(repeatingStreamLLMStub{chunk: "chunk", interval: 10 * time.Millisecond}))
	defer o.Close()

	responseReceived := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	cancellationCalls := atomic.Int32{}

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
			if cancellationCalls.Add(1) == 1 {
				select {
				case cancelled <- struct{}{}:
				default:
				}
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
	o.CancelTurn()

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for cancellation callback")
	}

	time.Sleep(50 * time.Millisecond)
	if got := cancellationCalls.Load(); got != 1 {
		t.Fatalf("expected exactly one cancellation callback, got %d", got)
	}
}

func TestInterruptionHandlingSeesUpdatedToolSet(t *testing.T) {
	handler := &toolSnapshotEventHandler{}
	o := NewOrchestrator(
		WithStreamingLLM(repeatingStreamLLMStub{chunk: "chunk", interval: 10 * time.Millisecond}),
		WithTools(testTool("tool_a")),
		WithEventHandlerV0(handler),
	)
	defer o.Close()

	responseReceived := make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o.Orchestrate(ctx,
		WithResponseCallback(func(string) {
			select {
			case responseReceived <- struct{}{}:
			default:
			}
		}),
	)

	o.SendPrompt("start")

	select {
	case <-responseReceived:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for first turn to start")
	}

	WithTools(testTool("tool_b"))(o)
	o.SendPrompt("interrupt")

	waitForCondition(t, 2*time.Second, "interruption tools snapshot", func() bool {
		names := handler.snapshot()
		return len(names) > 0
	})

	latestSnapshot := handler.latest()
	if len(latestSnapshot) == 0 || latestSnapshot[0] != "tool_b" {
		t.Fatalf("expected interruption handling to see updated tools [tool_b], got %v", latestSnapshot)
	}

	o.CancelTurn()
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

type scriptedStreamLLMStub struct {
	chunks   []string
	interval time.Duration
}

func (stub scriptedStreamLLMStub) PromptWithStream(context.Context, *string, ...llms.StreamingPromptOption) llms.Stream {
	return scriptedStreamStub{chunks: append([]string(nil), stub.chunks...), interval: stub.interval}
}

type scriptedStreamStub struct {
	chunks   []string
	interval time.Duration
}

func (stub scriptedStreamStub) Chunks(ctx context.Context) func(func(llms.StreamChunk, error) bool) {
	return func(yield func(llms.StreamChunk, error) bool) {
		for _, chunk := range stub.chunks {
			select {
			case <-ctx.Done():
				return
			case <-time.After(stub.interval):
			}

			if !yield(streamContentChunkStub{content: chunk}, nil) {
				return
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

type toolSnapshotEventHandler struct {
	mu    sync.Mutex
	names [][]string
}

func (handler *toolSnapshotEventHandler) HandleV0(_ context.Context, event llms.EventV0, conversation conversations.ActiveContextV0) iter.Seq2[llms.EventV0, error] {
	return func(yield func(llms.EventV0, error) bool) {
		if conversation.ActiveTurn() != nil {
			tools := conversation.AvailableTools()
			names := make([]string, 0, len(tools))
			for _, tool := range tools {
				names = append(names, tool.Function.Name)
			}

			handler.mu.Lock()
			handler.names = append(handler.names, names)
			handler.mu.Unlock()
		}

		yield(event, nil)
	}
}

func (handler *toolSnapshotEventHandler) snapshot() [][]string {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	cloned := make([][]string, len(handler.names))
	for i, entry := range handler.names {
		cloned[i] = append([]string(nil), entry...)
	}
	return cloned
}

func (handler *toolSnapshotEventHandler) latest() []string {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.names) == 0 {
		return nil
	}
	return append([]string(nil), handler.names[len(handler.names)-1]...)
}

func testTool(name string) llms.Tool {
	return llms.NewTool(name, "test tool", map[string]llms.ParameterBase{}, func(struct{}) (string, error) {
		return "ok", nil
	})
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
