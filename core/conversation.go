package orchestration

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/koscakluka/ema-core/core/conversations"
	"github.com/koscakluka/ema-core/core/llms"
)

var _ conversations.ActiveContextV0 = (*activeConversation)(nil)

type activeConversation struct {
	mu sync.RWMutex

	turns []llms.TurnV1

	activeTurn *activeTurn
	runtime    *conversationRuntime
}

func newConversation(runtime *conversationRuntime) activeConversation {
	return activeConversation{runtime: runtime}
}

// ConversationV1 is a point-in-time view of conversation state.
type ConversationV1 struct {
	History        []llms.TurnV1
	ActiveTurn     *llms.TurnV1
	AvailableTools []llms.Tool
}

func (t *activeConversation) Snapshot() ConversationV1 {
	t.mu.RLock()

	turns := make([]llms.TurnV1, len(t.turns))
	copy(turns, t.turns)

	var activeTurn *llms.TurnV1
	if t.activeTurn != nil {
		snapshot := t.activeTurn.TurnV1
		activeTurn = &snapshot
	}

	runtime := t.runtime
	t.mu.RUnlock()

	var tools []llms.Tool
	if runtime != nil {
		tools = runtime.llm.availableTools()
	}

	return ConversationV1{History: turns, ActiveTurn: activeTurn, AvailableTools: tools}
}

func (t *activeConversation) History() []llms.TurnV1 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	history := make([]llms.TurnV1, len(t.turns))
	copy(history, t.turns)
	return history
}

func (t *activeConversation) ActiveTurn() *llms.TurnV1 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.activeTurn == nil {
		return nil
	}

	snapshot := t.activeTurn.TurnV1
	return &snapshot
}

func (t *activeConversation) AvailableTools() []llms.Tool {
	runtime := t.runtimeSnapshot()
	if runtime == nil {
		return nil
	}

	return runtime.llm.availableTools()
}

func (t *activeConversation) ActiveTurnContext() context.Context {
	activeTurn := t.activeTurnRef()
	if activeTurn == nil {
		return nil
	}

	return activeTurn.ctx
}

func (t *activeConversation) IsActiveTurnCancelled() bool {
	activeTurn := t.activeTurnRef()
	if activeTurn == nil {
		return false
	}

	return activeTurn.IsCancelled()
}

func (t *activeConversation) CancelActiveTurn() bool {
	activeTurn := t.activeTurnRef()
	if activeTurn == nil || activeTurn.IsCancelled() {
		return false
	}

	activeTurn.Cancel()
	return true
}

func (t *activeConversation) pauseActiveTurn() bool {
	activeTurn := t.activeTurnRef()
	if activeTurn == nil {
		return false
	}

	activeTurn.Pause()
	return true
}

func (t *activeConversation) unpauseActiveTurn() bool {
	activeTurn := t.activeTurnRef()
	if activeTurn == nil {
		return false
	}

	activeTurn.Unpause()
	return true
}

func (t *activeConversation) stopSpeakingActiveTurn() bool {
	activeTurn := t.activeTurnRef()
	if activeTurn == nil {
		return false
	}

	activeTurn.StopSpeaking()
	return true
}

func (t *activeConversation) addInterruptionToActiveTurn(interruption llms.InterruptionV0) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.activeTurn == nil {
		return false
	}

	t.activeTurn.Interruptions = append(t.activeTurn.Interruptions, interruption)
	return true
}

func (t *activeConversation) finaliseActiveTurn(finalisedTurn llms.TurnV1) (activeTurnIDMismatch bool, activeTurnMissing bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.activeTurn == nil {
		t.turns = append(t.turns, finalisedTurn)
		return false, true
	}

	if t.activeTurn.TurnV1.ID != finalisedTurn.ID {
		t.turns = append(t.turns, finalisedTurn)
		return true, false
	}

	t.turns = append(t.turns, finalisedTurn)
	t.activeTurn = nil
	return false, false
}

func (t *activeConversation) updateInterruption(id int64, update func(*llms.InterruptionV0)) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.activeTurn != nil {
		for j, interruption := range t.activeTurn.Interruptions {
			if interruption.ID == id {
				update(&t.activeTurn.Interruptions[j])
				return
			}
		}
	}
	for i, turn := range slices.Backward(t.turns) {
		for j, interruption := range turn.Interruptions {
			if interruption.ID == id {
				update(&t.turns[i].Interruptions[j])
			}
		}
	}
}

func (t *activeConversation) Start() (started bool) {
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

func (t *activeConversation) End() {
	runtime := t.runtimeSnapshot()
	if runtime == nil {
		return
	}

	runtime.endOnce.Do(func() {
		close(runtime.closeCh)
		t.CancelActiveTurn()
	})
}

func (t *activeConversation) AwaitCompletion() {
	runtime := t.runtimeSnapshot()
	if runtime == nil {
		return
	}

	if runtime.started.Load() {
		<-runtime.done
	}
}

func (t *activeConversation) Enqueue(event llms.EventV0) bool {
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

func (t *activeConversation) activeTurnRef() *activeTurn {
	t.mu.RLock()
	activeTurn := t.activeTurn
	t.mu.RUnlock()
	return activeTurn
}

func (t *activeConversation) runtimeSnapshot() *conversationRuntime {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.runtime
}
