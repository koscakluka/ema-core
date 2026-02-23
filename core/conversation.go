package orchestration

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/koscakluka/ema-core/core/conversations"
	"github.com/koscakluka/ema-core/core/llms"
)

var _ conversations.ActiveContextV0 = (*activeConversation)(nil)

var (
	ErrActiveTurnIDMismatch = errors.New("active turn finalisation failed: turn IDs do not match")
	ErrActiveTurnMissing    = errors.New("active turn finalisation failed: active turn missing")
)

type activeConversation struct {
	mu sync.RWMutex

	turns      []llms.TurnV1
	activeTurn *activeTurn

	availableTools func() []llms.Tool

	// currentPipeline provides access to the active response pipeline.
	//
	// HACK: This is to allow beckwards compatibility with the old
	// conversation APIs. It is not a great idea to keep this in the
	// TODO: Remove after removing deprecated APIs
	currentPipeline func() *responsePipeline
}

func newConversation(currentPipeline func() *responsePipeline, availableTools func() []llms.Tool) activeConversation {
	return activeConversation{
		availableTools:  availableTools,
		currentPipeline: currentPipeline,
	}
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

	availableTools := t.availableTools
	t.mu.RUnlock()

	var tools []llms.Tool
	if availableTools != nil {
		tools = availableTools()
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
	t.mu.RLock()
	availableTools := t.availableTools
	t.mu.RUnlock()
	if availableTools == nil {
		return nil
	}

	return availableTools()
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

func (t *activeConversation) startNewTurn(trigger llms.TriggerV0) (*activeTurn, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.activeTurn != nil {
		return nil, fmt.Errorf("active turn already set")
	}

	t.activeTurn = newActiveTurn(trigger)
	return t.activeTurn, nil
}

func (t *activeConversation) finaliseTurn(finalisedTurn llms.TurnV1) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.activeTurn == nil {
		t.turns = append(t.turns, finalisedTurn)
		return ErrActiveTurnMissing
	}

	if t.activeTurn.TurnV1.ID != finalisedTurn.ID {
		t.turns = append(t.turns, finalisedTurn)
		return ErrActiveTurnIDMismatch
	}

	t.turns = append(t.turns, finalisedTurn)
	t.activeTurn = nil
	return nil
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

type activeTurn struct {
	llms.TurnV1

	finalResponse *llms.TurnResponseV0
}

func newActiveTurn(trigger llms.TriggerV0) *activeTurn {
	return &activeTurn{
		TurnV1: llms.TurnV1{
			ID:      uuid.NewString(),
			Trigger: trigger,
		},
		finalResponse: &llms.TurnResponseV0{},
	}
}

func (t *activeTurn) Finalise() {
	if t.IsFinalised {
		return
	}

	if t.finalResponse != nil {
		t.Responses = append(t.Responses, *t.finalResponse)
	}
	t.IsFinalised = true
}
