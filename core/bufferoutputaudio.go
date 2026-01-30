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

	allAudioLoaded bool

	audio               [][]byte
	audioConsumed       int
	audioPlayed         int
	audioPlayingStarted time.Time
	audioDone           bool
	audioTranscript     string
	audioSignal         *sync.Cond

	audioMarks []struct {
		name     string
		position int
	}
	audioMarksConsumed int

	paused       bool
	pausedSignal *sync.Cond

	cleared bool
}

func newAudioBuffer() *audioBuffer {
	return &audioBuffer{
		audioSignal:  sync.NewCond(&sync.Mutex{}),
		pausedSignal: sync.NewCond(&sync.Mutex{}),
		sampleRate:   1,
	}
}

func (b *audioBuffer) AddAudio(audio []byte) {
	b.audio = append(b.audio, audio)
	b.audioSignal.Broadcast()
}

func (b *audioBuffer) Audio(yield func(audio audioOrMark) bool) {
	for {
		for {
			if len(b.audio) == b.audioConsumed {
				break
			}
			for b.paused {
				b.pausedSignal.L.Lock()
				b.pausedSignal.Wait()
				b.pausedSignal.L.Unlock()
				if b.cleared {
					return
				}
			}
			audio := b.audio[b.audioConsumed]
			b.audioConsumed++
			if !yield(audioOrMark{Type: "audio", Audio: audio}) {
				return
			}
			if b.audioPlayingStarted.IsZero() {
				b.audioPlayingStarted = time.Now()
			}
			for ; b.audioMarksConsumed < len(b.audioMarks); b.audioMarksConsumed++ {
				if b.audioMarks[b.audioMarksConsumed].position > b.audioConsumed {
					break
				}
				if !yield(audioOrMark{Type: "mark", Mark: b.audioMarks[b.audioMarksConsumed].name}) {
					return
				}
			}
		}
		for ; b.audioMarksConsumed < len(b.audioMarks); b.audioMarksConsumed++ {
			if b.audioMarks[b.audioMarksConsumed].position > b.audioConsumed {
				break
			}
			if !yield(audioOrMark{Type: "mark", Mark: b.audioMarks[b.audioMarksConsumed].name}) {
				return
			}
		}
		if b.audioDone {
			return
		}
		b.audioSignal.L.Lock()
		b.audioSignal.Wait()
		b.audioSignal.L.Unlock()
		if b.cleared {
			return
		}
	}
}

func (b *audioBuffer) AudioMark(name string) {
	b.audioMarks = append(b.audioMarks, struct {
		name     string
		position int
	}{
		name:     name,
		position: len(b.audio),
	})
	b.audioSignal.Broadcast()
}

func (b *audioBuffer) MarkPlayed(name string) {
	for _, mark := range b.audioMarks {
		if mark.name == name {
			// "duration", audioDuration(b.audio[b.audioPlayed:mark.position], b.sampleRate),
			// "actual_duration", time.Since(b.audioPlayingStarted),
			b.audioTranscript += name
			b.audioPlayed = mark.position
			b.audioPlayingStarted = time.Now()
			b.audioDone = true
			if b.allAudioLoaded && b.audioPlayed == len(b.audio) {
				b.audioDone = true
				b.audioSignal.Broadcast()
			}
			break
		}
	}
}

func (b *audioBuffer) AllAudioLoaded() {
	b.allAudioLoaded = true
}

func (b *audioBuffer) PauseAudio() {
	if b.audioDone {
		return
	}

	b.paused = true
	// TODO: Account for the latency of the audio sink (i.e. time it takes from
	// when audio leaves the buffer to when it is actually played + the time
	// it takes for use to receive the information that the audio was played)
	// TODO: Consider identifying silences in the audio so we can continue from
	// there and make the unpausing seem smoother (as a human would do)
	playedDuration := time.Since(b.audioPlayingStarted)
	samplesPlayed := audioSamples(playedDuration, b.sampleRate)
	chunksPlayed := 0
	for _, chunk := range b.audio[b.audioPlayed:] {
		samplesPlayed -= len(chunk)
		if samplesPlayed < 0 {
			// TODO: See what to do with underplayed audio, so far it hasn't
			// been an issue
			break
		}
		chunksPlayed++
	}
	b.audioPlayed += chunksPlayed
	b.audioConsumed = b.audioPlayed
	for i, mark := range b.audioMarks {
		if mark.position > b.audioConsumed {
			b.audioMarksConsumed = i
			break
		}
	}
	b.pausedSignal.Broadcast()
}

func (b *audioBuffer) UnpauseAudio() {
	b.paused = false
	b.audioPlayingStarted = time.Time{}
	b.pausedSignal.Broadcast()
	b.audioSignal.Broadcast()
}

func (b *audioBuffer) Clear() {
	b.cleared = true
	b.pausedSignal.Broadcast()
	b.audioSignal.Broadcast()
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
