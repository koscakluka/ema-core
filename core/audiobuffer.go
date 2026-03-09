package orchestration

import (
	"bytes"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/koscakluka/ema-core/core/audio"
)

type audioBuffer struct {
	mu sync.Mutex

	encodingInfo audio.EncodingInfo

	audio                [][]byte
	allAudioLoaded       bool
	legacyAllAudioLoaded bool
	usingWithLegacyTTS   bool // TODO: Remove this once we can remove the old TTS version

	internalPlayhead int
	externalPlayhead int

	lastMarkTimestamp time.Time

	marks []audioBufferMark

	stopped bool
	paused  bool

	updateSignal chan struct{}
}

type audioBufferMark struct {
	ID          string
	position    int
	terminal    bool
	broadcasted bool
	confirmed   bool

	audio          []byte
	approxPlayhead int
}

func newAudioBuffer(encodingInfo audio.EncodingInfo) *audioBuffer {
	return &audioBuffer{
		encodingInfo: encodingInfo,
		updateSignal: make(chan struct{}, 1),
	}
}

func (b *audioBuffer) AddAudio(audio []byte) {
	b.mu.Lock()
	b.audio = append(b.audio, audio)
	b.mu.Unlock()
	b.signalUpdate()
}

func (b *audioBuffer) Audio(yield func(audio audioOrMark) bool) {
	firstStart := sync.Once{}
	for {
		for {
			if ok := b.waitIfPaused(); !ok {
				return
			}

			audio, ok := b.consumeNextChunk()
			if !ok {
				break
			}

			firstStart.Do(func() {
				time.Sleep(50 * time.Millisecond)
				b.StartedPlaying()
			})

			if !yield(audioOrMark{Type: "audio", Audio: audio}) {
				return
			}
			if ok := b.broadcastMarks(yield); !ok {
				return
			}
		}
		if ok := b.waitForNextAudio(yield); !ok {
			return
		}
	}
}

func (b *audioBuffer) waitIfPaused() (ok bool) {
	for {
		b.mu.Lock()
		paused := b.paused
		stopped := b.stopped
		b.mu.Unlock()

		if stopped {
			return false
		}
		if !paused {
			return true
		}

		<-b.updateSignal
	}
}

func (b *audioBuffer) consumeNextChunk() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.audio) <= b.internalPlayhead {
		return nil, false
	}

	audio := b.audio[b.internalPlayhead]
	b.internalPlayhead++
	return audio, true
}

func (b *audioBuffer) broadcastMarks(yield func(audioOrMark) bool) (ok bool) {
	b.mu.Lock()
	marksToBroadcast := []string{}
	for i, mark := range b.marks {
		if mark.confirmed || mark.broadcasted {
			continue
		} else if mark.position > b.internalPlayhead {
			break
		}

		b.marks[i].broadcasted = true
		marksToBroadcast = append(marksToBroadcast, mark.ID)
	}
	b.mu.Unlock()

	for _, markID := range marksToBroadcast {
		if !yield(audioOrMark{Type: "mark", Mark: markID}) {
			return false
		}
	}

	return true
}

func (b *audioBuffer) waitForNextAudio(yield func(audioOrMark) bool) (ok bool) {
	for {
		b.mu.Lock()
		noAudioAvailable := len(b.audio) == b.internalPlayhead
		stopped := b.stopped
		audioDone := b.audioDoneLocked()
		b.mu.Unlock()

		if !noAudioAvailable {
			return !(stopped || audioDone)
		}

		if stopped || audioDone {
			return false
		}

		<-b.updateSignal
		// HACK: This is only here because sometimes the mark arrives after the
		// audio has been fully played and it will make this an infinite
		// waiting loop
		if ok := b.broadcastMarks(yield); !ok {
			return false
		}
	}
}

// audioDoneLocked is safe to call from a locked context.
func (b *audioBuffer) audioDoneLocked() bool {

	return (b.allAudioLoaded || (b.usingWithLegacyTTS && b.legacyAllAudioLoaded)) &&
		b.externalPlayhead == len(b.audio)
}

// Mark appends a mark at the current audio position.
//
// When legacy TTS mode is active, terminal marks are used as an explicit
// end-of-stream signal to avoid treating a transient "last known" mark as
// final while more chunks may still arrive.
func (b *audioBuffer) Mark(isTerminal ...bool) {
	terminal := len(isTerminal) > 0 && isTerminal[0]

	b.mu.Lock()
	markStart := 0
	if len(b.marks) > 0 {
		markStart = b.marks[len(b.marks)-1].position
	}
	b.marks = append(b.marks, audioBufferMark{
		ID:       uuid.NewString(),
		position: len(b.audio),
		terminal: terminal,

		audio: bytes.Join(b.audio[markStart:], nil),
	})
	b.mu.Unlock()
	b.signalUpdate()
}

func (b *audioBuffer) ConfirmMark(id string) bool {
	b.mu.Lock()
	shouldSignal := false
	confirmed := false
	for i, mark := range b.marks {
		if mark.confirmed {
			continue
		} else if !mark.broadcasted {
			break
		}
		if mark.ID == id {
			// "duration", audioDuration(b.audio[b.audioPlayed:mark.position], b.sampleRate),
			// "actual_duration", time.Since(b.audioPlayingStarted),
			b.marks[i].confirmed = true
			confirmed = true
			b.externalPlayhead = mark.position
			b.startedPlayingLocked()
			if (b.allAudioLoaded ||
				// HACK: Following condition is purely for using old tts interface
				// TODO: Remove this once we can remove the old TTS version
				(b.usingWithLegacyTTS && i == len(b.marks)-1 && b.marks[i].terminal)) &&
				b.externalPlayhead == len(b.audio) {
				b.legacyAllAudioLoaded = true
				shouldSignal = true
			}
			break
		}
	}
	b.mu.Unlock()

	if shouldSignal {
		b.signalUpdate()
	}

	return confirmed
}

func (b *audioBuffer) StartedPlaying() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.startedPlayingLocked()
}

// startedPlayingLocked is a version of [audioBuffer.StartedPlaying] that is safe to call from
// a locked context.
func (b *audioBuffer) startedPlayingLocked() {
	b.lastMarkTimestamp = time.Now()
	// TODO: It would also be good to trigger a timer in case marks fail and
	// we have to terminate the loop when we think the audio was supposed to end
	// this seems to sometimes happen
	// Account for latency and pausing + all audio needs to be loaded to trigger
	// this, it needs to be run here because we are using this after the
	// audio resumes
}

func (b *audioBuffer) AllAudioLoaded() {
	b.mu.Lock()
	b.allAudioLoaded = true
	b.mu.Unlock()
	b.signalUpdate()
	// TODO: Start timer to automatically terminate playing after audio is
	// supposed to have ended
}

func (b *audioBuffer) SetUsingLegacyTTSMode() {
	b.mu.Lock()
	b.usingWithLegacyTTS = true
	b.mu.Unlock()
}

func (b *audioBuffer) Pause() {
	b.mu.Lock()
	if b.audioDoneLocked() || b.paused {
		b.mu.Unlock()
		return
	}

	b.rewindLocked()
	b.paused = true
	b.mu.Unlock()
	b.signalUpdate()
}

func (b *audioBuffer) rewindLocked() {
	// TODO: Account for the latency of the audio sink (i.e. time it takes from
	// when audio leaves the buffer to when it is actually played + the time
	// it takes for use to receive the information that the audio was played)
	// TODO: Consider identifying silences in the audio so we can continue from
	// there and make the unpausing seem smoother (as a human would do)

	approxPlayhead := min(b.externalPlayhead, b.internalPlayhead)

	if !b.paused && !b.stopped && !b.lastMarkTimestamp.IsZero() {
		playedBytes := b.encodingInfo.BytesForDuration(time.Since(b.lastMarkTimestamp))
		for i := approxPlayhead; i < b.internalPlayhead && playedBytes > 0; i++ {
			chunkBytes := len(b.audio[i])
			if playedBytes < chunkBytes {
				break
			}
			playedBytes -= chunkBytes
			approxPlayhead++
		}
	}

	b.externalPlayhead = approxPlayhead
	b.internalPlayhead = approxPlayhead
	for i, mark := range b.marks {
		if mark.position > approxPlayhead {
			b.marks[i].broadcasted = false
		}
	}
}

func (b *audioBuffer) Resume() {
	b.mu.Lock()
	if b.audioDoneLocked() || !b.paused {
		b.mu.Unlock()
		return
	}

	b.paused = false
	b.startedPlayingLocked()
	b.mu.Unlock()
	b.signalUpdate()
}

func (b *audioBuffer) Stop() {
	b.mu.Lock()
	if b.stopped {
		b.mu.Unlock()
		return
	}
	b.stopped = true
	b.mu.Unlock()
	b.signalUpdate()
}

func (b *audioBuffer) signalUpdate() {
	select {
	case b.updateSignal <- struct{}{}:
	default:
	}
}

type audioOrMark struct {
	Type  string
	Audio []byte
	Mark  string
}

const (
	audioOrMarkTypeAudio = "audio"
	audioOrMarkTypeMark  = "mark"
)

func (b *audioBuffer) Progress(markIdx int) (float64, []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.lastMarkTimestamp.IsZero() {
		return 0, nil
	}

	if b.paused || b.stopped {
		// TODO: We should probably return the progress here as well as any
		// audio we haven't played yet.
		return 0, nil
	}

	audioFrame := []byte{}
	for i, mark := range b.marks {
		if mark.approxPlayhead == len(mark.audio) {
			if markIdx == i {
				return 1, audioFrame
			}
			continue
		}

		if markIdx == i {
			playedSamplesSinceMark := b.encodingInfo.BytesForDuration(time.Since(b.lastMarkTimestamp))
			if playedSamplesSinceMark <= 0 ||
				playedSamplesSinceMark < mark.approxPlayhead {
				return 0, audioFrame
			}

			playedSamplesSinceMark = min(playedSamplesSinceMark, len(mark.audio))

			audioFrame = append(audioFrame, mark.audio[mark.approxPlayhead:playedSamplesSinceMark]...)
			mark.approxPlayhead = playedSamplesSinceMark

			return float64(playedSamplesSinceMark) / float64(len(mark.audio)), audioFrame
		}

		audioFrame = append(audioFrame, mark.audio[mark.approxPlayhead:]...)
		b.marks[i].approxPlayhead = len(mark.audio)
	}

	return 0, audioFrame
}
