package orchestration

import (
	"context"
	"slices"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Turns struct {
	turns []llms.Turn
	// TODO: Consider adding ID to turns to be able to find the active turn
	// if needed instead of keeping track of an index

	// activeTurnIdx is the index of the active turn
	//
	// it is an int so that active turn can be correctly modified even if the
	// underlying slice changes
	activeTurnIdx int
	activeTurnCtx context.Context
}

// Push adds a new turn to the stored turns
func (t *Turns) Push(turn llms.Turn) {
	t.turns = append(t.turns, turn)
}

// Pop removes the last turn from the stored turns, returns nil if empty
func (t *Turns) Pop() *llms.Turn {
	if len(t.turns) == 0 {
		return nil
	}
	lastElementIdx := len(t.turns) - 1
	turn := t.turns[lastElementIdx]
	t.turns = t.turns[:lastElementIdx]
	if t.activeTurnIdx == lastElementIdx {
		t.activeTurnIdx = -1
	}
	return &turn
}

// Clear removes all stored turns
func (t *Turns) Clear() {
	t.turns = nil
	t.activeTurnIdx = -1
}

// Values is an iterator that goes over all the stored turns starting from the
// earliest towards the latest
func (t *Turns) Values(yield func(llms.Turn) bool) {
	for _, turn := range t.turns {
		if !yield(turn) {
			return
		}
	}
}

// Values is an iterator that goes over all the stored turns starting from the
// latest towards the earliest
func (t *Turns) RValues(yield func(llms.Turn) bool) {
	// TODO: There should be a better way to do this than creating a new
	// method just for reversing the order
	for _, turn := range slices.Backward(t.turns) {
		if !yield(turn) {
			return
		}
	}
}

func (t *Turns) pushActiveTurn(ctx context.Context, turn llms.Turn) {
	t.activeTurnCtx = ctx
	t.activeTurnIdx = len(t.turns)
	t.turns = append(t.turns, turn)
}

func (t *Turns) activeTurn() *llms.Turn {
	if t.activeTurnIdx < 0 || t.activeTurnIdx >= len(t.turns) {
		return nil
	}
	return &t.turns[t.activeTurnIdx]
}

func (t *Turns) updateActiveTurn(turn llms.Turn) {
	if t.activeTurnIdx < 0 || t.activeTurnIdx >= len(t.turns) {
		return
	}

	t.turns[t.activeTurnIdx] = turn
}

func (t *Turns) unsetActiveTurn() {
	t.activeTurnIdx = -1
	t.activeTurnCtx = nil
}

func (o *Orchestrator) finaliseActiveTurn() {
	activeTurn := o.turns.activeTurn()
	if activeTurn != nil {
		span := trace.SpanFromContext(o.turns.activeTurnCtx)
		interruptionTypes := []string{}
		for _, interruption := range activeTurn.Interruptions {
			interruptionTypes = append(interruptionTypes, interruption.Type)
		}
		span.SetAttributes(attribute.StringSlice("assistant_turn.interruptions", interruptionTypes))
		activeTurn.Stage = llms.TurnStageFinalized
		span.End()
		o.turns.updateActiveTurn(*activeTurn)
		o.turns.unsetActiveTurn()
	}
}

func (t *Turns) addInterruption(interruption llms.InterruptionV0) {
	activeTurn := t.activeTurn()
	if activeTurn != nil {
		activeTurn.Interruptions = append(activeTurn.Interruptions, interruption)
		t.updateActiveTurn(*activeTurn)
	}
}

func (t *Turns) findInterruption(id int64) *llms.InterruptionV0 {
	for turn := range t.RValues {
		for _, interruption := range turn.Interruptions {
			if interruption.ID == id {
				return &interruption
			}
		}
	}

	return nil
}

func (t *Turns) updateInterruption(id int64, update func(*llms.InterruptionV0)) {
	for i, turn := range slices.Backward(t.turns) {
		for j, interruption := range turn.Interruptions {
			if interruption.ID == id {
				update(&t.turns[i].Interruptions[j])
			}
		}
	}
}
