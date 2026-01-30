package orchestration

import (
	"sync"
	"time"
)

// TODO: Calculate the error factor based on marks' timestamps
const errorFactor = 0.5

// TODO: Optimize memory at some point, it is not a great idea to just append
// to a slice when we already consumed a part of it. But it needs to be synced
// properly, probably a ring buffer makes sense.

type audioBuffer struct {
	sampleRate int

	audio          [][]byte
	allAudioLoaded bool

	internalPlayhead int
	externalPlayhead int

	lastMarkTimestamp time.Time
	newAudioSignal    *sync.Cond

	marks []struct {
		name     string
		position int
	}
	audioMarksConsumed int

	stopped      bool
	paused       bool
	resumeSignal *sync.Cond
}

func newAudioBuffer() *audioBuffer {
	return &audioBuffer{
		newAudioSignal: sync.NewCond(&sync.Mutex{}),
		resumeSignal:   sync.NewCond(&sync.Mutex{}),
		sampleRate:     1,
	}
}

func (b *audioBuffer) AddAudio(audio []byte) {
	b.audio = append(b.audio, audio)
	b.newAudioSignal.Broadcast()
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
			// TODO: Move outside the buffer, it should be communicated from
			// the outside where there is more information of when it actually
			// started
			// It would also be good to trigger a timer in case marks fail and
			// we have to terminate the loop when the audio was supposed to end
			if b.lastMarkTimestamp.IsZero() {
				b.lastMarkTimestamp = time.Now()
			}
			for ; b.audioMarksConsumed < len(b.marks); b.audioMarksConsumed++ {
				if b.marks[b.audioMarksConsumed].position > b.internalPlayhead {
					break
				}
				if !yield(audioOrMark{Type: "mark", Mark: b.marks[b.audioMarksConsumed].name}) {
					return
				}
			}
		}
		// TODO: Why is this here? It doesn't make any sense, even if we break
		// it was due to already having broadcasted the marks
		for ; b.audioMarksConsumed < len(b.marks); b.audioMarksConsumed++ {
			if b.marks[b.audioMarksConsumed].position > b.internalPlayhead {
				break
			}
			if !yield(audioOrMark{Type: "mark", Mark: b.marks[b.audioMarksConsumed].name}) {
				return
			}
		}
		if ok := b.waitForNextAudio(); !ok {
			return
		}
	}
}

func (b *audioBuffer) waitIfPaused() (ok bool) {
	for b.paused {
		if b.stopped {
			return false
		}

		b.resumeSignal.L.Lock()
		b.resumeSignal.Wait()
		b.resumeSignal.L.Unlock()
	}

	return !b.stopped
}

func (b *audioBuffer) consumeNextChunk() []byte {
	audio := b.audio[b.internalPlayhead]
	b.internalPlayhead++
	return audio
}

func (b *audioBuffer) waitForNextAudio() (ok bool) {
	for len(b.audio) == b.internalPlayhead {
		if b.stopped || b.audioDone() {
			return false
		}
		b.newAudioSignal.L.Lock()
		b.newAudioSignal.Wait()
		b.newAudioSignal.L.Unlock()
	}
	return !(b.stopped || b.audioDone())
}

func (b *audioBuffer) audioDone() bool {
	return b.allAudioLoaded && b.externalPlayhead == len(b.audio)
}

func (b *audioBuffer) Mark(name string) {
	// TODO: Expand marks to contain IDs so it can be used to identify the mark
	// without sharing information
	// Also add bools: broadcasted, confirmed
	b.marks = append(b.marks, struct {
		name     string
		position int
	}{
		name:     name,
		position: len(b.audio),
	})
	b.newAudioSignal.Broadcast()
}

// TODO: Rename to ConfirmMark
func (b *audioBuffer) ConfirmMark(name string) {
	for _, mark := range b.marks {
		if mark.name == name {
			// "duration", audioDuration(b.audio[b.audioPlayed:mark.position], b.sampleRate),
			// "actual_duration", time.Since(b.audioPlayingStarted),
			b.externalPlayhead = mark.position
			b.lastMarkTimestamp = time.Now()
			if b.allAudioLoaded && b.externalPlayhead == len(b.audio) {
				b.newAudioSignal.Broadcast()
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
	// Account for latency and pausing
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
	// NOTE This is necessary because we might be stuck in newAudioSignal.Wait
	// if everything has already been played
	b.newAudioSignal.Broadcast()
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
			b.audioMarksConsumed = i
			break
		}
	}
}

func (b *audioBuffer) Resume() {
	if b.audioDone() || !b.paused {
		return
	}

	b.paused = false
	b.StartedPlaying()
	b.resumeSignal.Broadcast()
}

func (b *audioBuffer) Stop() {
	b.stopped = true
	b.resumeSignal.Broadcast()
	b.newAudioSignal.Broadcast()
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
