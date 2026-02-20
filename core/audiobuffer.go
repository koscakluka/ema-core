package orchestration

import (
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
	transcript  string
	position    int
	broadcasted bool
	confirmed   bool
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

func (b *audioBuffer) audioDone() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.audioDoneLocked()
}

// audioDoneLocked is a version of [audioBuffer.audioDone] that is safe to call
// from a locked context.
func (b *audioBuffer) audioDoneLocked() bool {

	return (b.allAudioLoaded || (b.usingWithLegacyTTS && b.legacyAllAudioLoaded)) &&
		b.externalPlayhead == len(b.audio)
}

func (b *audioBuffer) Mark(transcript string) {
	b.mu.Lock()
	b.marks = append(b.marks, audioBufferMark{
		ID:         uuid.NewString(),
		transcript: transcript,
		position:   len(b.audio),
	})
	b.mu.Unlock()
	b.signalUpdate()
}

func (b *audioBuffer) GetMarkText(id string) *string {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i := range b.marks {
		if b.marks[i].ID == id {
			transcript := b.marks[i].transcript
			return &transcript
		}
	}
	return nil
}

func (b *audioBuffer) ConfirmMark(id string) {
	b.mu.Lock()
	shouldSignal := false
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
			b.externalPlayhead = mark.position
			b.startedPlayingLocked()
			if (b.allAudioLoaded ||
				// HACK: Following condition is purely for using old tts interface
				// TODO: Remove this once we can remove the old TTS version
				(b.usingWithLegacyTTS && i == len(b.marks)-1 && b.marks[i].transcript == "")) &&
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

func (b *audioBuffer) Pause() {
	b.mu.Lock()
	if b.audioDoneLocked() || b.paused {
		b.mu.Unlock()
		return
	}

	b.paused = true
	b.rewindLocked()
	b.mu.Unlock()
	b.signalUpdate()
}

func (b *audioBuffer) rewindLocked() {
	// TODO: Account for the latency of the audio sink (i.e. time it takes from
	// when audio leaves the buffer to when it is actually played + the time
	// it takes for use to receive the information that the audio was played)
	// TODO: Consider identifying silences in the audio so we can continue from
	// there and make the unpausing seem smoother (as a human would do)
	playedDuration := time.Since(b.lastMarkTimestamp)
	samplesPlayed := audioSamples(playedDuration, b.encodingInfo)
	chunksPlayed := 0
	for _, chunk := range b.audio[b.externalPlayhead:] {
		samplesPlayed -= len(chunk)
		if samplesPlayed < 0 {
			// TODO: See what to do with underplayed audio, so far it hasn't
			// been an issue
			break
		}
		chunksPlayed++
	}
	b.externalPlayhead += chunksPlayed
	b.internalPlayhead = b.externalPlayhead
	for i, mark := range b.marks {
		if mark.position > b.internalPlayhead {
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

func audioLen(audio [][]byte) int {
	chunksTotalLength := 0
	for _, audioChunk := range audio {
		chunksTotalLength += len(audioChunk)
	}
	return chunksTotalLength
}

func audioDuration(audio [][]byte, encodingInfo audio.EncodingInfo) time.Duration {
	return time.Duration(float64(audioLen(audio)) / float64(encodingInfo.SampleRate) * float64(time.Second) / float64(encodingInfo.Format.ByteSize()))
}

func audioSamples(duration time.Duration, encodingInfo audio.EncodingInfo) int {
	return int(float64(duration) / float64(time.Second) * float64(encodingInfo.SampleRate) * float64(encodingInfo.Format.ByteSize()))
}
