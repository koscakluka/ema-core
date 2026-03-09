package orchestration

import (
	"bytes"
	"testing"
	"time"

	"github.com/koscakluka/ema-core/core/audio"
)

func TestApproximatePlaybackDeltaInterpolatesAndClampsToConsumedAudio(t *testing.T) {
	b := newAudioBufferForTest()
	b.AddAudio([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	b.Mark()

	progress, frame := b.Progress(0)
	if progress != 0 {
		t.Fatalf("expected zero progress before playback starts, got %f", progress)
	}
	if len(frame) != 0 {
		t.Fatalf("expected no frame before playback starts, got %v", frame)
	}
}

func TestApproximateCurrentSegmentProgressInterpolatesToNextMark(t *testing.T) {
	b := newAudioBufferForTest()
	segment := make([]byte, 100)
	b.AddAudio(segment)
	b.Mark()
	b.StartedPlaying()

	time.Sleep(250 * time.Millisecond)

	progress, frame := b.Progress(0)
	if progress <= 0 || progress >= 1 {
		t.Fatalf("expected in-flight progress for current segment, got %f", progress)
	}
	if len(frame) == 0 || len(frame) >= len(segment) {
		t.Fatalf("expected partial frame while segment is in flight, got %d bytes", len(frame))
	}
}

func TestApproximateCurrentSegmentProgressReturnsZeroWithoutNextMark(t *testing.T) {
	b := newAudioBufferForTest()
	segment := make([]byte, 40)
	b.AddAudio(segment)
	b.Mark()
	b.StartedPlaying()
	time.Sleep(300 * time.Millisecond)

	progress, frame := b.Progress(0)
	if progress != 1 {
		t.Fatalf("expected progress to clamp at 1 after segment duration, got %f", progress)
	}
	if len(frame) != len(segment) {
		t.Fatalf("expected full segment frame at completion, got %d bytes", len(frame))
	}
}

func TestApproximateCurrentSegmentProgressAndNextUpdateUsesChunkDuration(t *testing.T) {
	b := newAudioBufferForTest()
	firstSegment := []byte{1, 2, 3, 4}
	secondSegment := []byte{9, 8, 7, 6, 5, 4}
	b.AddAudio(firstSegment)
	b.Mark()
	b.AddAudio(secondSegment)
	b.Mark()
	b.StartedPlaying()

	progress, frame := b.Progress(1)
	if progress < 0 || progress > 1 {
		t.Fatalf("expected bounded progress for later segment, got %f", progress)
	}
	if !bytes.HasPrefix(frame, firstSegment) {
		t.Fatalf("expected frame to flush earlier mark audio first, got %v", frame)
	}

	_, nextFrame := b.Progress(1)
	if bytes.HasPrefix(nextFrame, firstSegment) {
		t.Fatalf("expected flushed earlier mark audio to not repeat, got %v", nextFrame)
	}
}

func TestApproximateCurrentSegmentProgressAndNextUpdateFallsBackWhenPaused(t *testing.T) {
	b := newAudioBufferForTest()
	b.AddAudio(make([]byte, 10))
	b.Mark()
	b.StartedPlaying()
	b.Pause()

	progress, frame := b.Progress(0)
	if progress != 0 {
		t.Fatalf("expected paused segment progress 0, got %f", progress)
	}
	if len(frame) != 0 {
		t.Fatalf("expected paused progress to emit no frame, got %v", frame)
	}
}

func TestApproximatePlaybackDeltaReturnsAppendOnlyDelta(t *testing.T) {
	b := newAudioBufferForTest()
	b.AddAudio([]byte{1, 2, 3, 4})
	b.Mark()
	b.StartedPlaying()

	time.Sleep(300 * time.Millisecond)

	progress, frame := b.Progress(0)
	if progress != 1 {
		t.Fatalf("expected completed progress for first mark, got %f", progress)
	}
	if !bytes.Equal(frame, []byte{1, 2, 3, 4}) {
		t.Fatalf("expected emitted frame to equal mark audio, got %v", frame)
	}
}

func TestApproximatePlaybackDeltaSkipsRegression(t *testing.T) {
	b := newAudioBufferForTest()
	b.AddAudio([]byte{1, 2, 3, 4})
	b.Mark()
	b.StartedPlaying()
	time.Sleep(100 * time.Millisecond)
	b.Pause()

	progress, frame := b.Progress(0)
	if progress != 0 {
		t.Fatalf("expected paused playback to report zero progress, got %f", progress)
	}
	if len(frame) != 0 {
		t.Fatalf("expected paused playback to emit no frame, got %v", frame)
	}
}

func TestConfirmMarkLegacyModeDoesNotFinishForNonTerminalMark(t *testing.T) {
	b := newAudioBufferForTest()
	b.SetUsingLegacyTTSMode()
	b.AddAudio([]byte{1, 2, 3})
	b.Mark()
	defer b.Stop()

	marks, done := streamMarksAsync(b)

	var markID string
	select {
	case markID = <-marks:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for emitted mark")
	}

	if ok := b.ConfirmMark(markID); !ok {
		t.Fatalf("expected mark to be confirmed")
	}

	select {
	case <-done:
		t.Fatalf("expected playback to keep waiting for more audio in legacy non-terminal mode")
	case <-time.After(150 * time.Millisecond):
	}

	b.Stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for playback to stop")
	}
}

func TestConfirmMarkLegacyModeFinishesForTerminalMark(t *testing.T) {
	b := newAudioBufferForTest()
	b.SetUsingLegacyTTSMode()
	b.AddAudio([]byte{1, 2, 3})
	b.Mark(true)
	defer b.Stop()

	marks, done := streamMarksAsync(b)

	var markID string
	select {
	case markID = <-marks:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for emitted mark")
	}

	if ok := b.ConfirmMark(markID); !ok {
		t.Fatalf("expected mark to be confirmed")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("expected terminal legacy mark to finish playback")
	}
}

func newAudioBufferForTest() *audioBuffer {
	return newAudioBuffer(audio.EncodingInfo{SampleRate: 100, Format: audio.EncodingLinear16})
}

func streamMarksAsync(b *audioBuffer) (<-chan string, <-chan struct{}) {
	marks := make(chan string, 4)
	done := make(chan struct{})

	go func() {
		defer close(marks)
		defer close(done)
		b.Audio(func(item audioOrMark) bool {
			if item.Type == audioOrMarkTypeMark {
				marks <- item.Mark
			}
			return true
		})
	}()

	return marks, done
}
