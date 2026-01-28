package orchestration

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Turns struct {
	turns []llms.Turn
	// TODO: Consider adding ID to turns to be able to find the active turn
	// if needed instead of keeping track of an index

	activeTurn *activeTurn
}

// Push adds a new turn to the stored turns
func (t *Turns) Push(turn llms.Turn) {
	t.turns = append(t.turns, turn)
}

// Pop removes the last turn from the stored turns, returns nil if empty
func (t *Turns) Pop() *llms.Turn {
	if activeTurn := t.activeTurn; activeTurn != nil {
		activeTurn.Cancelled = true
		t.activeTurn = nil
		return &activeTurn.Turn
	}

	if len(t.turns) == 0 {
		return nil
	}
	lastElementIdx := len(t.turns) - 1
	turn := t.turns[lastElementIdx]
	t.turns = t.turns[:lastElementIdx]
	return &turn
}

// Clear removes all stored turns
func (t *Turns) Clear() {
	t.turns = nil
	t.activeTurn = nil
}

// Values is an iterator that goes over all the stored turns starting from the
// earliest towards the latest
func (t *Turns) Values(yield func(llms.Turn) bool) {
	for _, turn := range t.turns {
		if !yield(turn) {
			return
		}
	}
	if activeTurn := t.activeTurn; activeTurn != nil {
		if !yield(activeTurn.Turn) {
			return
		}
	}
}

// Values is an iterator that goes over all the stored turns starting from the
// latest towards the earliest
func (t *Turns) RValues(yield func(llms.Turn) bool) {
	if activeTurn := t.activeTurn; activeTurn != nil {
		if !yield(activeTurn.Turn) {
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

func (t *Turns) updateInterruption(id int64, update func(*llms.InterruptionV0)) {
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

func (t *Turns) startActiveTurn(ctx context.Context, components activeTurnComponents, callbacks activeTurnCallbacks, config activeTurnConfig) error {
	// TODO: active turn needs a mutex (not really but it would be nice)
	if t.activeTurn != nil {
		return fmt.Errorf("active turn already set")
	}

	t.activeTurn = newActiveTurn(ctx, components, callbacks, config)
	go t.activeTurn.processResponseText()
	go t.activeTurn.processSpeech()

	t.activeTurn.generateResponse()

	return nil
}
