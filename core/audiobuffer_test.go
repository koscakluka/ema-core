package orchestration

import (
	"bytes"
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

func TestApproximatePlaybackDeltaReturnsAppendOnlyDelta(t *testing.T) {
	b := newAudioBuffer(audio.EncodingInfo{SampleRate: 10, Format: audio.EncodingLinear16})
	b.AddAudio([]byte{1, 2})
	b.AddAudio([]byte{3, 4})

	b.mu.Lock()
	b.externalPlayhead = 0
	b.internalPlayhead = 2
	b.lastMarkTimestamp = time.Now().Add(-2 * time.Second)
	b.mu.Unlock()

	delta, playhead, _ := b.ApproximatePlaybackDelta(0)
	if !bytes.Equal(delta, []byte{1, 2, 3, 4}) {
		t.Fatalf("expected combined playback delta %v, got %v", []byte{1, 2, 3, 4}, delta)
	}
	if playhead != 2 {
		t.Fatalf("expected playhead 2 after first delta, got %d", playhead)
	}

	nextDelta, nextPlayhead, _ := b.ApproximatePlaybackDelta(playhead)
	if len(nextDelta) != 0 {
		t.Fatalf("expected no second delta after catch-up, got %v", nextDelta)
	}
	if nextPlayhead != 2 {
		t.Fatalf("expected playhead to remain 2, got %d", nextPlayhead)
	}
}

func TestApproximatePlaybackDeltaSkipsRegression(t *testing.T) {
	b := newAudioBuffer(audio.EncodingInfo{SampleRate: 10, Format: audio.EncodingLinear16})
	b.AddAudio([]byte{1, 2})
	b.AddAudio([]byte{3, 4})

	b.mu.Lock()
	b.externalPlayhead = 0
	b.internalPlayhead = 2
	b.lastMarkTimestamp = time.Now().Add(-2 * time.Second)
	b.mu.Unlock()

	_, playhead, _ := b.ApproximatePlaybackDelta(0)
	if playhead != 2 {
		t.Fatalf("expected initial playhead 2, got %d", playhead)
	}

	b.mu.Lock()
	b.paused = true
	b.externalPlayhead = 0
	b.internalPlayhead = 1
	b.mu.Unlock()

	delta, regressedPlayhead, _ := b.ApproximatePlaybackDelta(playhead)
	if len(delta) != 0 {
		t.Fatalf("expected regression to emit no delta, got %v", delta)
	}
	if regressedPlayhead != playhead {
		t.Fatalf("expected playhead to stay %d on regression, got %d", playhead, regressedPlayhead)
	}
}

func TestConfirmMarkLegacyModeDoesNotFinishForNonTerminalMark(t *testing.T) {
	b := newAudioBuffer(audio.EncodingInfo{SampleRate: 10, Format: audio.EncodingLinear16})
	b.SetUsingLegacyTTSMode()
	b.AddAudio([]byte{1, 2, 3})
	b.Mark()

	b.mu.Lock()
	if len(b.marks) != 1 {
		b.mu.Unlock()
		t.Fatalf("expected exactly one mark")
	}
	markID := b.marks[0].ID
	b.marks[0].broadcasted = true
	b.mu.Unlock()

	if ok := b.ConfirmMark(markID); !ok {
		t.Fatalf("expected mark to be confirmed")
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.legacyAllAudioLoaded {
		t.Fatalf("expected legacy completion to stay false for non-terminal mark")
	}
}

func TestConfirmMarkLegacyModeFinishesForTerminalMark(t *testing.T) {
	b := newAudioBuffer(audio.EncodingInfo{SampleRate: 10, Format: audio.EncodingLinear16})
	b.SetUsingLegacyTTSMode()
	b.AddAudio([]byte{1, 2, 3})
	b.Mark(true)

	b.mu.Lock()
	if len(b.marks) != 1 {
		b.mu.Unlock()
		t.Fatalf("expected exactly one mark")
	}
	markID := b.marks[0].ID
	b.marks[0].broadcasted = true
	b.mu.Unlock()

	if ok := b.ConfirmMark(markID); !ok {
		t.Fatalf("expected mark to be confirmed")
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.legacyAllAudioLoaded {
		t.Fatalf("expected legacy completion to become true for terminal mark")
	}
}
