package orchestration

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// TODO: Calculate the error factor based on marks' timestamps,
// NOTE: This is probably caused by the fact that the array we pass is byte,
// and the expected encoding is linear16, which is 2 bytes.
const errorFactor = 0.5

type audioBuffer struct {
	sampleRate int

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

func newAudioBuffer() *audioBuffer {
	return &audioBuffer{
		sampleRate:   1,
		updateSignal: make(chan struct{}, 1),
	}
}

func (b *audioBuffer) AddAudio(audio []byte) {
	b.audio = append(b.audio, audio)
	b.signalUpdate()
}

func (b *audioBuffer) Audio(yield func(audio audioOrMark) bool) {
	firstStart := sync.Once{}
	for {
		for len(b.audio) > b.internalPlayhead {
			if ok := b.waitIfPaused(); !ok {
				return
			}
			firstStart.Do(b.StartedPlaying)
			if !yield(audioOrMark{Type: "audio", Audio: b.consumeNextChunk()}) {
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
	for b.paused {
		if b.stopped {
			return false
		}
		<-b.updateSignal
	}

	return !b.stopped
}

func (b *audioBuffer) consumeNextChunk() []byte {
	audio := b.audio[b.internalPlayhead]
	b.internalPlayhead++
	return audio
}

func (b *audioBuffer) broadcastMarks(yield func(audioOrMark) bool) (ok bool) {
	for i, mark := range b.marks {
		if mark.confirmed || mark.broadcasted {
			continue
		} else if mark.position > b.internalPlayhead {
			break
		}

		b.marks[i].broadcasted = true
		if !yield(audioOrMark{Type: "mark", Mark: mark.ID}) {
			return false
		}
	}

	return true
}

func (b *audioBuffer) waitForNextAudio(yield func(audioOrMark) bool) (ok bool) {
	for len(b.audio) == b.internalPlayhead {
		if b.stopped || b.audioDone() {
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
	return !(b.stopped || b.audioDone())
}

func (b *audioBuffer) audioDone() bool {
	return (b.allAudioLoaded || (b.usingWithLegacyTTS && b.legacyAllAudioLoaded)) &&
		b.externalPlayhead == len(b.audio)
}

func (b *audioBuffer) Mark(transcript string) {
	b.marks = append(b.marks, audioBufferMark{
		ID:         uuid.NewString(),
		transcript: transcript,
		position:   len(b.audio),
	})
	b.signalUpdate()
}

func (b *audioBuffer) ConfirmMark(id string) {
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
			b.StartedPlaying()
			if (b.allAudioLoaded ||
				// HACK: Following condition is purely for using old tts interface
				// TODO: Remove this once we can remove the old TTS version
				(b.usingWithLegacyTTS && i == len(b.marks)-1 && b.marks[i].transcript == "")) &&
				b.externalPlayhead == len(b.audio) {
				b.legacyAllAudioLoaded = true
				b.signalUpdate()
			}
			break
		}
	}
}

func (b *audioBuffer) StartedPlaying() {
	b.lastMarkTimestamp = time.Now()
	// TODO: It would also be good to trigger a timer in case marks fail and
	// we have to terminate the loop when we think the audio was supposed to end
	// this seems to sometimes happen
	// Account for latency and pausing + all audio needs to be loaded to trigger
	// this, it needs to be run here because we are using this after the
	// audio resumes
}

func (b *audioBuffer) AllAudioLoaded() {
	b.allAudioLoaded = true
	// TODO: Start timer to automatically terminate playing after audio is
	// supposed to have ended
}

func (b *audioBuffer) Pause() {
	if b.audioDone() || b.paused {
		return
	}

	b.paused = true
	b.rewind()
	b.signalUpdate()
}

func (b *audioBuffer) rewind() {
	// TODO: Account for the latency of the audio sink (i.e. time it takes from
	// when audio leaves the buffer to when it is actually played + the time
	// it takes for use to receive the information that the audio was played)
	// TODO: Consider identifying silences in the audio so we can continue from
	// there and make the unpausing seem smoother (as a human would do)
	playedDuration := time.Since(b.lastMarkTimestamp)
	samplesPlayed := audioSamples(playedDuration, b.sampleRate)
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
	if b.audioDone() || !b.paused {
		return
	}

	b.paused = false
	b.StartedPlaying()
	b.signalUpdate()
}

func (b *audioBuffer) Stop() {
	b.stopped = true
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

func audioDuration(audio [][]byte, sampleRate int) time.Duration {
	return time.Duration(float64(audioLen(audio)) / float64(sampleRate) * float64(time.Second) * errorFactor)
}

func audioSamples(duration time.Duration, sampleRate int) int {
	return int(float64(duration) / float64(time.Second) * float64(sampleRate) / errorFactor)
}
