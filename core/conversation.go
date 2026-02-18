package orchestration

import (
	"context"
	"slices"
	"sync"

	"github.com/koscakluka/ema-core/core/llms"
)

type Conversation struct {
	mu sync.RWMutex

	turns []llms.TurnV1

	activeTurn *activeTurn
	runtime    *conversationRuntime
}

func (t *Conversation) historySnapshot() []llms.TurnV1 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	history := make([]llms.TurnV1, len(t.turns))
	copy(history, t.turns)
	return history
}

func (t *Conversation) activeTurnSnapshot() *llms.TurnV1 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.activeTurn == nil {
		return nil
	}

	snapshot := t.activeTurn.TurnV1
	return &snapshot
}

func (t *Conversation) activeTurnContext() context.Context {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.activeTurn == nil {
		return nil
	}

	return t.activeTurn.ctx
}

func (t *Conversation) activeTurnCancelled() bool {
	t.mu.RLock()
	activeTurn := t.activeTurn
	t.mu.RUnlock()

	if activeTurn == nil {
		return false
	}

	return activeTurn.IsCancelled()
}

func (t *Conversation) cancelActiveTurn() bool {
	t.mu.RLock()
	activeTurn := t.activeTurn
	t.mu.RUnlock()

	if activeTurn == nil || activeTurn.IsCancelled() {
		return false
	}

	activeTurn.Cancel()
	return true
}

func (t *Conversation) pauseActiveTurn() bool {
	t.mu.RLock()
	activeTurn := t.activeTurn
	t.mu.RUnlock()

	if activeTurn == nil {
		return false
	}

	activeTurn.Pause()
	return true
}

func (t *Conversation) unpauseActiveTurn() bool {
	t.mu.RLock()
	activeTurn := t.activeTurn
	t.mu.RUnlock()

	if activeTurn == nil {
		return false
	}

	activeTurn.Unpause()
	return true
}

func (t *Conversation) stopSpeakingActiveTurn() bool {
	t.mu.RLock()
	activeTurn := t.activeTurn
	t.mu.RUnlock()

	if activeTurn == nil {
		return false
	}

	activeTurn.StopSpeaking()
	return true
}

func (t *Conversation) addInterruptionToActiveTurn(interruption llms.InterruptionV0) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.activeTurn == nil {
		return false
	}

	t.activeTurn.Interruptions = append(t.activeTurn.Interruptions, interruption)
	return true
}

func (t *Conversation) finaliseActiveTurn(finalisedTurn llms.TurnV1) (activeTurnIDMismatch bool, activeTurnMissing bool) {
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

func (t *Conversation) appendTurns(turns ...llms.TurnV1) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.turns = append(t.turns, turns...)
}

// Pop removes the last turn from the stored turns, returns nil if empty
func (t *Conversation) Pop() *llms.TurnV1 {
	t.mu.Lock()
	if activeTurn := t.activeTurn; activeTurn != nil {
		t.activeTurn = nil
		turn := activeTurn.TurnV1
		t.mu.Unlock()

		activeTurn.Cancel()
		return &turn
	}

	if len(t.turns) == 0 {
		t.mu.Unlock()
		return nil
	}
	lastElementIdx := len(t.turns) - 1
	turn := t.turns[lastElementIdx]
	t.turns = t.turns[:lastElementIdx]
	t.mu.Unlock()

	return &turn
}

func (t *Conversation) popOld() *llms.Turn {
	t.mu.RLock()
	activeTurn := t.activeTurn
	t.mu.RUnlock()

	if activeTurn != nil {
		activeTurn.Cancel()
		turns := llms.ToTurnsV0FromV1([]llms.TurnV1{activeTurn.TurnV1})
		if len(turns) > 1 {
			t.mu.Lock()
			if t.activeTurn == activeTurn {
				t.activeTurn.Responses = nil
				t.activeTurn.ToolCalls = nil
				t.activeTurn.Interruptions = nil
				t.activeTurn.IsFinalised = false
			}
			t.mu.Unlock()
			return &turns[1]
		}
		t.mu.Lock()
		if t.activeTurn == activeTurn {
			t.activeTurn = nil
		}
		t.mu.Unlock()
		return &turns[0]
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.turns) == 0 {
		return nil
	}
	lastElementIdx := len(t.turns) - 1
	turn := t.turns[lastElementIdx]
	turns := llms.ToTurnsV0FromV1([]llms.TurnV1{turn})
	if len(turns) > 1 {
		t.turns[lastElementIdx].Responses = nil
		t.turns[lastElementIdx].ToolCalls = nil
		t.turns[lastElementIdx].Interruptions = nil
		t.turns[lastElementIdx].IsFinalised = false
		return &turns[1]
	}
	t.turns = t.turns[:lastElementIdx]
	return &turns[0]
}

// Clear removes all stored turns
func (t *Conversation) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.turns = nil
	t.activeTurn = nil
}

// Values is an iterator that goes over all the stored turns starting from the
// earliest towards the latest
func (t *Conversation) Values(yield func(llms.TurnV1) bool) {
	for _, turn := range t.historySnapshot() {
		if !yield(turn) {
			return
		}
	}
	if activeTurn := t.activeTurnSnapshot(); activeTurn != nil {
		if !yield(*activeTurn) {
			return
		}
	}
}

// Values is an iterator that goes over all the stored turns starting from the
// latest towards the earliest
func (t *Conversation) RValues(yield func(llms.TurnV1) bool) {
	if activeTurn := t.activeTurnSnapshot(); activeTurn != nil {
		if !yield(*activeTurn) {
			return
		}
	}
	// TODO: There should be a better way to do this than creating a new
	// method just for reversing the order
	for _, turn := range slices.Backward(t.historySnapshot()) {
		if !yield(turn) {
			return
		}
	}
}

func (t *Conversation) updateInterruption(id int64, update func(*llms.InterruptionV0)) {
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
