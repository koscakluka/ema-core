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

func TestApproximateCurrentSegmentProgressLockedInterpolatesToNextMark(t *testing.T) {
	b := newAudioBuffer(audio.EncodingInfo{SampleRate: 10, Format: audio.EncodingLinear16})
	b.AddAudio(make([]byte, 10))
	b.AddAudio(make([]byte, 10))
	b.AddAudio(make([]byte, 10))

	now := time.Now()
	b.mu.Lock()
	b.externalPlayhead = 1
	b.internalPlayhead = 3
	b.lastMarkTimestamp = now.Add(-600 * time.Millisecond)
	b.marks = []audioBufferMark{{ID: "m1", position: 3}}
	got := b.approximateCurrentSegmentProgressLocked(now)
	b.mu.Unlock()

	if got != 0.5 {
		t.Fatalf("expected segment progress 0.5, got %f", got)
	}
}

func TestApproximateCurrentSegmentProgressLockedReturnsZeroWithoutNextMark(t *testing.T) {
	b := newAudioBuffer(audio.EncodingInfo{SampleRate: 10, Format: audio.EncodingLinear16})
	b.AddAudio(make([]byte, 10))
	b.AddAudio(make([]byte, 10))

	now := time.Now()
	b.mu.Lock()
	b.externalPlayhead = 1
	b.internalPlayhead = 2
	b.lastMarkTimestamp = now.Add(-600 * time.Millisecond)
	got := b.approximateCurrentSegmentProgressLocked(now)
	b.mu.Unlock()

	if got != 0 {
		t.Fatalf("expected segment progress 0 without next mark, got %f", got)
	}
}

func TestApproximateCurrentSegmentProgressAndNextUpdateLockedUsesChunkDuration(t *testing.T) {
	b := newAudioBuffer(audio.EncodingInfo{SampleRate: 10, Format: audio.EncodingLinear16})
	b.AddAudio(make([]byte, 10))
	b.AddAudio(make([]byte, 10))
	b.AddAudio(make([]byte, 10))

	now := time.Now()
	b.mu.Lock()
	b.externalPlayhead = 1
	b.internalPlayhead = 3
	b.lastMarkTimestamp = now.Add(-600 * time.Millisecond)
	b.marks = []audioBufferMark{{ID: "m1", position: 3}}
	progress, nextUpdate := b.approximateCurrentSegmentProgressAndNextUpdateLocked(now)
	b.mu.Unlock()

	if progress != 0.5 {
		t.Fatalf("expected segment progress 0.5, got %f", progress)
	}
	if nextUpdate != 400*time.Millisecond {
		t.Fatalf("expected next update in 400ms, got %s", nextUpdate)
	}
}

func TestApproximateCurrentSegmentProgressAndNextUpdateLockedFallsBackWhenPaused(t *testing.T) {
	b := newAudioBuffer(audio.EncodingInfo{SampleRate: 10, Format: audio.EncodingLinear16})
	b.AddAudio(make([]byte, 10))

	now := time.Now()
	b.mu.Lock()
	b.paused = true
	b.externalPlayhead = 0
	b.internalPlayhead = 1
	b.lastMarkTimestamp = now.Add(-100 * time.Millisecond)
	progress, nextUpdate := b.approximateCurrentSegmentProgressAndNextUpdateLocked(now)
	b.mu.Unlock()

	if progress != 0 {
		t.Fatalf("expected paused segment progress 0, got %f", progress)
	}
	if nextUpdate != defaultApproximateUpdateDelay {
		t.Fatalf("expected fallback update delay %s, got %s", defaultApproximateUpdateDelay, nextUpdate)
	}
}
