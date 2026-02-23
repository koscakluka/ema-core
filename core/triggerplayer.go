package orchestration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type triggerPlayer struct {
	// TODO: Queue should be owned by the conversation, it is an indicator of
	// future work that might happen, and it should probably just be a slice,
	// it is not processed that fast that it matters. + it could be useful
	// to the trigger processor when triggers are received.
	queue   chan triggerQueueItem
	closeCh chan struct{}
	done    chan struct{}

	startOnce sync.Once
	endOnce   sync.Once

	started atomic.Bool

	onCancel func()
}

func newTriggerPlayer() *triggerPlayer {
	return &triggerPlayer{
		queue:   make(chan triggerQueueItem, conversationTriggerQueueCapacity), // TODO: Figure out good values for this.
		closeCh: make(chan struct{}),
		done:    make(chan struct{}),

		onCancel: func() {},
	}
}

func (b *triggerPlayer) SetOnCancel(onCancel func()) {
	if b == nil {
		return
	}

	if onCancel != nil {
		b.onCancel = onCancel
	}
}

func (b *triggerPlayer) CanIngest() bool {
	if b == nil {
		return false
	}

	select {
	case <-b.closeCh:
		return false
	default:
		return true
	}
}

func (loop *triggerPlayer) StartLoop(baseCtx context.Context, startNewTurn func(context.Context, llms.TriggerV0) error) (started bool) {
	if loop == nil || startNewTurn == nil || !loop.CanIngest() {
		return false
	}

	loop.startOnce.Do(func() {
		if !loop.CanIngest() {
			return
		}

		started = true
		loop.started.Store(true)
		go func() {
			defer close(loop.done)

			for {
				select {
				case <-loop.closeCh:
					return
				case queuedTrigger := <-loop.queue:
					if !loop.CanIngest() {
						return
					}
					loop.processQueuedTrigger(baseCtx, queuedTrigger, startNewTurn)
				}
			}
		}()
	})

	return started
}

func (loop *triggerPlayer) Stop() {
	if loop == nil {
		return
	}

	loop.endOnce.Do(func() { close(loop.closeCh) })
}

func (loop *triggerPlayer) Clear() {
	if loop == nil {
		return
	}

	for {
		select {
		case <-loop.queue:
		default:
			return
		}
	}
}

func (loop *triggerPlayer) AwaitDone() {
	if loop == nil {
		return
	}

	if loop.started.Load() {
		<-loop.done
	}
}

type triggerQueueItem struct {
	trigger  llms.TriggerV0
	queuedAt time.Time
}

func (loop *triggerPlayer) Ingest(trigger llms.TriggerV0) bool {
	if loop == nil || !loop.CanIngest() {
		return false
	}

	queueItem := triggerQueueItem{trigger: trigger, queuedAt: time.Now()}
	select {
	case <-loop.closeCh:
		return false
	case loop.queue <- queueItem:
		return true
	}
}

func (loop *triggerPlayer) processQueuedTrigger(
	baseContext context.Context,
	queuedTrigger triggerQueueItem,
	startNewTurn func(context.Context, llms.TriggerV0) error,
) {
	if loop == nil || startNewTurn == nil {
		return
	}

	turnCtx, turnCancel := context.WithCancel(baseContext)
	defer turnCancel()

	go func() {
		select {
		case <-loop.closeCh:
			turnCancel()
		case <-turnCtx.Done():
		}
	}()

	ctx, span := tracer.Start(turnCtx, "process turn")
	defer span.End()

	queuedTime := time.Since(queuedTrigger.queuedAt).Seconds()
	span.AddEvent("taken out of queue", trace.WithAttributes(attribute.Float64("assistant_turn.queued_time", queuedTime)))
	span.SetAttributes(attribute.Float64("assistant_turn.queued_time", queuedTime))

	trigger := queuedTrigger.trigger

	if err := startNewTurn(ctx, trigger); err != nil {
		err := fmt.Errorf("failed to start new turn: %v", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		// TODO: Probably should be able to requeue the prompt or something
		// here
	}
}

func (loop *triggerPlayer) queuedTriggerCount() int {
	if loop == nil {
		return 0
	}

	return len(loop.queue)
}

func (loop *triggerPlayer) OnCancel() {
	if loop == nil {
		return
	}

	loop.onCancel()
}
