package orchestration

import (
	"testing"
	"time"

	"github.com/koscakluka/ema-core/core/audio"
)

func TestApproximatePlayheadLockedInterpolatesFromExternalPlayhead(t *testing.T) {
	b := newAudioBuffer(audio.EncodingInfo{SampleRate: 10, Format: audio.EncodingLinear16})
	b.AddAudio(make([]byte, 10))
	b.AddAudio(make([]byte, 10))
	b.AddAudio(make([]byte, 10))

	now := time.Now()
	b.mu.Lock()
	b.externalPlayhead = 1
	b.internalPlayhead = 3
	b.lastMarkTimestamp = now.Add(-time.Second)
	got := b.approximatePlayheadLocked(now)
	b.mu.Unlock()

	if got != 3 {
		t.Fatalf("expected approximate playhead to advance to 3, got %d", got)
	}
}

func TestApproximatePlayheadLockedClampsToInternalPlayhead(t *testing.T) {
	b := newAudioBuffer(audio.EncodingInfo{SampleRate: 10, Format: audio.EncodingLinear16})
	b.AddAudio(make([]byte, 10))

	now := time.Now()
	b.mu.Lock()
	b.externalPlayhead = 0
	b.internalPlayhead = 1
	b.lastMarkTimestamp = now.Add(-5 * time.Second)
	got := b.approximatePlayheadLocked(now)
	b.mu.Unlock()

	if got != 1 {
		t.Fatalf("expected approximate playhead to clamp at internal playhead 1, got %d", got)
	}
}

func TestApproximatePlayheadLockedStopsWhenPaused(t *testing.T) {
	b := newAudioBuffer(audio.EncodingInfo{SampleRate: 10, Format: audio.EncodingLinear16})
	b.AddAudio(make([]byte, 10))
	b.AddAudio(make([]byte, 10))
	b.AddAudio(make([]byte, 10))

	now := time.Now()
	b.mu.Lock()
	b.externalPlayhead = 1
	b.internalPlayhead = 3
	b.lastMarkTimestamp = now.Add(-time.Second)
	b.paused = true
	got := b.approximatePlayheadLocked(now)
	b.mu.Unlock()

	if got != 1 {
		t.Fatalf("expected approximate playhead to stay at external playhead 1 while paused, got %d", got)
	}
}
