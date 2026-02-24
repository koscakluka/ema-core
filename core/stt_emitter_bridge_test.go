package orchestration

import (
	"context"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/koscakluka/ema-core/core/conversations"
	events "github.com/koscakluka/ema-core/core/events"
	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/core/triggers"
)

func TestComposeSTTEventEmitterForwardsEventsAndRoutesTriggers(t *testing.T) {
	o := NewOrchestrator()
	defer o.Close()

	handler := &capturingSTTTriggerHandler{}
	o.triggerHandler = handler

	forwardedKinds := []events.Kind{}
	emit := o.composeSTTEventEmitter(func(event events.Event) {
		forwardedKinds = append(forwardedKinds, event.Kind())
	})

	emit(events.NewUserSpeechStarted())
	emit(events.NewUserTranscriptInterimSegmentUpdated("he"))
	emit(events.NewUserTranscriptInterimUpdated("he"))
	emit(events.NewUserTranscriptInterimUpdated(""))
	emit(events.NewUserTranscriptFinal("hello"))
	emit(events.NewUserSpeechEnded())

	if len(forwardedKinds) != 6 {
		t.Fatalf("expected all events to be forwarded, got %d", len(forwardedKinds))
	}

	deadline := time.Now().Add(2 * time.Second)
	for len(handler.snapshot()) < 4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	seenStarted := 0
	seenEnded := 0
	seenInterim := 0
	seenFinal := 0
	for _, trigger := range handler.snapshot() {
		switch trigger.(type) {
		case triggers.SpeechStartedTrigger:
			seenStarted++
		case triggers.SpeechEndedTrigger:
			seenEnded++
		case triggers.InterimTranscriptionTrigger:
			seenInterim++
		case triggers.TranscriptionTrigger:
			seenFinal++
		}
	}

	if seenStarted != 1 || seenEnded != 1 || seenInterim != 1 || seenFinal != 1 {
		t.Fatalf(
			"expected one trigger each for speech started/ended, non-empty interim, and final transcription, got started=%d ended=%d interim=%d final=%d",
			seenStarted,
			seenEnded,
			seenInterim,
			seenFinal,
		)
	}
}

type capturingSTTTriggerHandler struct {
	mu       sync.Mutex
	triggers []llms.TriggerV0
}

func (h *capturingSTTTriggerHandler) HandleTriggerV0(
	_ context.Context,
	trigger llms.TriggerV0,
	_ conversations.ActiveContextV0,
) iter.Seq2[llms.TriggerV0, error] {
	h.mu.Lock()
	h.triggers = append(h.triggers, trigger)
	h.mu.Unlock()

	return func(func(llms.TriggerV0, error) bool) {}
}

func (h *capturingSTTTriggerHandler) snapshot() []llms.TriggerV0 {
	h.mu.Lock()
	defer h.mu.Unlock()

	cloned := make([]llms.TriggerV0, len(h.triggers))
	copy(cloned, h.triggers)
	return cloned
}
