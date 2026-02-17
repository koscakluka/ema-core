package orchestration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/koscakluka/ema-core/core/llms"
)

type Conversation struct {
	turns []llms.TurnV1

	activeTurn *activeTurn
}

// Pop removes the last turn from the stored turns, returns nil if empty
func (t *Conversation) Pop() *llms.TurnV1 {
	if activeTurn := t.activeTurn; activeTurn != nil {
		activeTurn.Cancel()
		t.activeTurn = nil
		return &activeTurn.TurnV1
	}

	if len(t.turns) == 0 {
		return nil
	}
	lastElementIdx := len(t.turns) - 1
	turn := t.turns[lastElementIdx]
	t.turns = t.turns[:lastElementIdx]
	return &turn
}

func (t *Conversation) popOld() *llms.Turn {
	if activeTurn := t.activeTurn; activeTurn != nil {
		activeTurn.Cancel()
		turns := llms.ToTurnsV0FromV1([]llms.TurnV1{activeTurn.TurnV1})
		if len(turns) > 1 {
			t.activeTurn.Responses = nil
			t.activeTurn.ToolCalls = nil
			t.activeTurn.Interruptions = nil
			t.activeTurn.IsFinalised = false
			return &turns[1]
		}
		t.activeTurn = nil
		return &turns[0]
	}

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
	t.turns = nil
	t.activeTurn = nil
}

// Values is an iterator that goes over all the stored turns starting from the
// earliest towards the latest
func (t *Conversation) Values(yield func(llms.TurnV1) bool) {
	for _, turn := range t.turns {
		if !yield(turn) {
			return
		}
	}
	if activeTurn := t.activeTurn; activeTurn != nil {
		if !yield(activeTurn.TurnV1) {
			return
		}
	}
}

// Values is an iterator that goes over all the stored turns starting from the
// latest towards the earliest
func (t *Conversation) RValues(yield func(llms.TurnV1) bool) {
	if activeTurn := t.activeTurn; activeTurn != nil {
		if !yield(activeTurn.TurnV1) {
			return
		}
	}
	// TODO: There should be a better way to do this than creating a new
	// method just for reversing the order
	for _, turn := range slices.Backward(t.turns) {
		if !yield(turn) {
			return
		}
	}
}

func (t *Conversation) updateInterruption(id int64, update func(*llms.InterruptionV0)) {
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

func (t *Conversation) processActiveTurn(ctx context.Context, event llms.EventV0, components activeTurnComponents, callbacks activeTurnCallbacks, config activeTurnConfig) error {
	// TODO: active turn needs a mutex (not really but it would be nice)
	if t.activeTurn != nil {
		return fmt.Errorf("active turn already set")
	}

	activeTurn := newActiveTurn(ctx, event, components, callbacks, config)
	t.activeTurn = activeTurn
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
		run("response generation", func(ctx context.Context) error {
			return activeTurn.generateResponse(ctx, t.turns)
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
		return nil
	}()
	addWorkerErr(finaliseErr)

	if workerErr != nil {
		return fmt.Errorf("one or more active turn processes failed: %w", workerErr)
	}

	return nil
}
