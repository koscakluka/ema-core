package orchestration

import (
	"context"
	"fmt"
	"slices"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/attribute"
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

func (t *Turns) setActiveTurn(ctx context.Context, turn llms.Turn, audioOutput audioOutput) error {
	if t.activeTurn != nil {
		return fmt.Errorf("active turn already set")
	}

	t.activeTurn = &activeTurn{
		ctx:         ctx,
		Turn:        turn,
		textBuffer:  *newTextBuffer(),
		audioBuffer: *newAudioBuffer(),
	}

	if audioOutput != nil {
		t.activeTurn.audioBuffer.sampleRate = audioOutput.EncodingInfo().SampleRate
	}

	return nil
}

func (o *Orchestrator) finaliseActiveTurn() {
	activeTurn := o.turns.activeTurn
	if activeTurn != nil {
		span := trace.SpanFromContext(activeTurn.ctx)
		interruptionTypes := []string{}
		for _, interruption := range activeTurn.Interruptions {
			interruptionTypes = append(interruptionTypes, interruption.Type)
		}
		span.SetAttributes(attribute.StringSlice("assistant_turn.interruptions", interruptionTypes))
		span.SetAttributes(attribute.Int("assistant_turn.queued_triggers", len(o.transcripts)))
		span.End()
		o.turns.turns = append(o.turns.turns, activeTurn.Turn)
		o.turns.activeTurn = nil
		o.promptEnded.Done()
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

type activeTurn struct {
	llms.Turn

	ctx         context.Context
	textBuffer  textBuffer
	audioBuffer audioBuffer
}

func (t *activeTurn) AddInterruption(interruption llms.InterruptionV0) error {
	if t.Cancelled {
		return fmt.Errorf("turn cancelled")
	} else if t.Stage == llms.TurnStageFinalized {
		return fmt.Errorf("turn already finalized")
	}

	t.Interruptions = append(t.Interruptions, interruption)
	return nil
}

func (t *activeTurn) Pause() {
	t.audioBuffer.PauseAudio()
}

func (t *activeTurn) Unpause() {
	t.audioBuffer.UnpauseAudio()
}

func (t *activeTurn) IsMutable() bool {
	return !t.IsFinalized()
}

func (t *activeTurn) IsFinalized() bool {
	return t.Stage == llms.TurnStageFinalized || t.Cancelled
}
