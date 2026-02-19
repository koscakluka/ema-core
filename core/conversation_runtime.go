package orchestration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const conversationEventQueueCapacity = 10

type eventQueueItem struct {
	event    llms.EventV0
	queuedAt time.Time
}

type runtimeCallbacks struct {
	onResponse     func(string)
	onResponseEnd  func()
	onAudio        func([]byte)
	onAudioEnded   func(string)
	onCancellation func()
}

type conversationRuntime struct {
	baseContext context.Context
	// llm is the model facade for prompt and stream execution.
	llm llm
	// textToSpeech is the facade that normalizes legacy and v1 TTS clients.
	textToSpeech textToSpeech
	// audioOutput is the facade that normalizes output marks/audio delivery.
	audioOutput audioOutput
	callbacks   runtimeCallbacks

	queue   chan eventQueueItem
	closeCh chan struct{}
	done    chan struct{}

	startOnce sync.Once
	endOnce   sync.Once

	started  atomic.Bool
	speaking atomic.Bool
}

func newConversationRuntime() *conversationRuntime {
	runtime := &conversationRuntime{
		llm:         newLLM(),
		baseContext: context.Background(),
		queue:       make(chan eventQueueItem, conversationEventQueueCapacity), // TODO: Figure out good values for this.
		closeCh:     make(chan struct{}),
		done:        make(chan struct{}),
	}
	return runtime
}

func (runtime *conversationRuntime) configure(ctx context.Context, callbacks runtimeCallbacks) {
	if runtime == nil {
		return
	}

	runtime.baseContext = ctx
	runtime.callbacks = callbacks
}

func (runtime *conversationRuntime) setSpeaking(isSpeaking bool) {
	if runtime == nil {
		return
	}

	runtime.speaking.Store(isSpeaking)
}

func (runtime *conversationRuntime) speakingEnabled() bool {
	if runtime == nil {
		return false
	}

	return runtime.speaking.Load()
}

func (t *Conversation) setRuntime(runtime *conversationRuntime) *conversationRuntime {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.runtime == nil {
		t.runtime = runtime
	}
	return t.runtime
}

func (t *Conversation) runtimeSnapshot() *conversationRuntime {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.runtime
}

func (t *Conversation) start() (started bool) {
	runtime := t.runtimeSnapshot()
	if runtime == nil || runtime.isClosed() {
		return false
	}

	runtime.startOnce.Do(func() {
		if runtime.isClosed() {
			return
		}

		started = true
		runtime.started.Store(true)
		go func() {
			defer close(runtime.done)

			for {
				select {
				case <-runtime.closeCh:
					return
				case queuedEvent := <-runtime.queue:
					if runtime.isClosed() {
						return
					}
					runtime.processQueuedEvent(t, queuedEvent)
				}
			}
		}()
	})

	return started
}

func (t *Conversation) end() {
	runtime := t.runtimeSnapshot()
	if runtime == nil {
		return
	}

	runtime.endOnce.Do(func() {
		close(runtime.closeCh)
		t.cancelActiveTurn()
	})
}

func (t *Conversation) waitUntilEnded() {
	runtime := t.runtimeSnapshot()
	if runtime == nil {
		return
	}

	if runtime.started.Load() {
		<-runtime.done
	}
}

func (t *Conversation) enqueue(event llms.EventV0) bool {
	runtime := t.runtimeSnapshot()
	if runtime == nil {
		// TODO: Decide what to do with events queued before runtime starts.
		return false
	}

	if runtime.isClosed() {
		return false
	}

	queueItem := eventQueueItem{event: event, queuedAt: time.Now()}
	select {
	case <-runtime.closeCh:
		return false
	case runtime.queue <- queueItem:
		return true
	}
}

func (runtime *conversationRuntime) isClosed() bool {
	if runtime == nil {
		return false
	}

	select {
	case <-runtime.closeCh:
		return true
	default:
		return false
	}
}

func (runtime *conversationRuntime) queuedEventCount() int {
	if runtime == nil {
		return 0
	}

	return len(runtime.queue)
}

func (runtime *conversationRuntime) processActiveTurn(
	ctx context.Context,
	conversation *Conversation,
	event llms.EventV0,
	onFinalise func(*activeTurn),
) error {
	if runtime == nil || conversation == nil {
		return fmt.Errorf("runtime and conversation are required")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var workerErr error
	workerErrMu := sync.Mutex{}
	addWorkerErr := func(err error) {
		if err == nil {
			return
		}
		workerErrMu.Lock()
		workerErr = errors.Join(workerErr, err)
		workerErrMu.Unlock()
	}

	isSpeaking := runtime.speakingEnabled()

	conversation.mu.Lock()
	if conversation.activeTurn != nil {
		conversation.mu.Unlock()
		return fmt.Errorf("active turn already set")
	}

	activeTurn := newActiveTurn(
		ctx,
		event,
		&runtime.llm,
		runtime.textToSpeech.client(),
		runtime.audioOutput.Snapshot(),
		isSpeaking,
		runtime.callbacks.onResponse,
		runtime.callbacks.onResponseEnd,
		runtime.callbacks.onAudio,
		runtime.callbacks.onAudioEnded,
		runtime.callbacks.onCancellation,
	)
	history := make([]llms.TurnV1, len(conversation.turns))
	copy(history, conversation.turns)
	conversation.activeTurn = activeTurn
	conversation.mu.Unlock()

	run := func(name string, f func(context.Context) error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				addWorkerErr(fmt.Errorf("%s worker panicked: %v", name, recovered))
				cancel()
			}
		}()

		if err := f(ctx); err != nil {
			addWorkerErr(fmt.Errorf("%s worker failed: %w", name, err))
			cancel()
		}
	}

	wg := &sync.WaitGroup{}
	wg.Add(3)
	go func() {
		defer wg.Done()
		run("llm generation", func(ctx context.Context) error {
			return activeTurn.generateLLM(ctx, history)
		})
	}()
	go func() {
		defer wg.Done()
		run("response text processing", activeTurn.processResponseText)
	}()
	go func() {
		defer wg.Done()
		run("speech processing", activeTurn.processSpeech)
	}()

	wg.Wait()

	finaliseErr := func() (err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("active turn finalise panicked: %v", recovered)
			}
		}()

		activeTurn.Finalise()

		activeTurnIDMismatch, activeTurnMissing := conversation.finaliseActiveTurn(activeTurn.TurnV1)
		if activeTurnIDMismatch {
			addWorkerErr(fmt.Errorf("active turn finalisation failed: turn IDs do not match"))
		}
		if activeTurnMissing {
			addWorkerErr(fmt.Errorf("active turn finalisation failed: active turn missing"))
		}

		if onFinalise != nil {
			onFinalise(activeTurn)
		}

		return nil
	}()
	addWorkerErr(finaliseErr)

	if workerErr != nil {
		return fmt.Errorf("one or more active turn processes failed: %w", workerErr)
	}

	return nil
}

func (runtime *conversationRuntime) processQueuedEvent(conversation *Conversation, queuedEvent eventQueueItem) {
	if runtime == nil || conversation == nil {
		return
	}

	turnCtx, turnCancel := context.WithCancel(runtime.baseContext)
	defer turnCancel()

	go func() {
		select {
		case <-runtime.closeCh:
			turnCancel()
		case <-turnCtx.Done():
		}
	}()

	ctx, span := tracer.Start(turnCtx, "process turn")
	defer span.End()

	queuedTime := time.Since(queuedEvent.queuedAt).Seconds()
	span.AddEvent("taken out of queue", trace.WithAttributes(attribute.Float64("assistant_turn.queued_time", queuedTime)))
	span.SetAttributes(attribute.Float64("assistant_turn.queued_time", queuedTime))

	event := queuedEvent.event

	onFinalise := func(finalisedTurn *activeTurn) {
		interruptionTypes := []string{}
		for _, interruption := range finalisedTurn.Interruptions {
			interruptionTypes = append(interruptionTypes, interruption.Type)
		}
		span.SetAttributes(attribute.StringSlice("assistant_turn.interruptions", interruptionTypes))
		span.SetAttributes(attribute.Int("assistant_turn.queued_events", runtime.queuedEventCount()))
	}
	if err := runtime.processActiveTurn(ctx, conversation, event, onFinalise); err != nil {
		err := fmt.Errorf("failed to process active turn: %v", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		// TODO: Probably should be able to requeue the prompt or something
		// here
	}
}
