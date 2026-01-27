package orchestration

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/koscakluka/ema-core/core/texttospeech"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (o *Orchestrator) initTTS() {
	if o.textToSpeechClient != nil {
		ttsOptions := []texttospeech.TextToSpeechOption{
			texttospeech.WithAudioCallback(o.outputAudioBuffer.AddAudio),
			texttospeech.WithAudioEndedCallback(o.outputAudioBuffer.AudioMark),
		}
		if o.audioOutput != nil {
			ttsOptions = append(ttsOptions, texttospeech.WithEncodingInfo(o.audioOutput.EncodingInfo()))
		}

		if err := o.textToSpeechClient.OpenStream(context.TODO(), ttsOptions...); err != nil {
			log.Printf("Failed to open deepgram speech stream: %v", err)
		}
	}
}

func (o *Orchestrator) passSpeechToAudioOutput(ctx context.Context) {
	_, span := tracer.Start(ctx, "passing speech to audio output")
	defer span.End()
bufferReadingLoop:
	for audioOrMark := range o.outputAudioBuffer.Audio {
		switch audioOrMark.Type {
		case "audio":
			audio := audioOrMark.Audio
			if o.orchestrateOptions.onAudio != nil {
				o.orchestrateOptions.onAudio(audio)
			}

			if o.audioOutput == nil {
				continue bufferReadingLoop
			}

			if !o.IsSpeaking || (o.turns.activeTurn() != nil && o.turns.activeTurn().Cancelled) {
				o.audioOutput.ClearBuffer()
				break bufferReadingLoop
			}

			o.audioOutput.SendAudio(audio)

		case "mark":
			mark := audioOrMark.Mark
			span.AddEvent("received mark", trace.WithAttributes(attribute.String("mark", mark)))
			if o.audioOutput != nil {
				switch o.audioOutput.(type) {
				case AudioOutputV1:
					o.audioOutput.(AudioOutputV1).Mark(mark, func(mark string) {
						o.outputAudioBuffer.MarkPlayed(mark)
					})
				case AudioOutputV0:
					go func() {
						o.audioOutput.(AudioOutputV0).AwaitMark()
						o.outputAudioBuffer.MarkPlayed(mark)
					}()
				}
			} else {
				o.outputAudioBuffer.MarkPlayed(mark)
			}
		}
	}

	defer func() {
		o.finaliseActiveTurn()
		o.promptEnded.Done()
	}()

	if o.orchestrateOptions.onAudioEnded != nil {
		o.orchestrateOptions.onAudioEnded(o.outputAudioBuffer.audioTranscript)
	}

	if o.audioOutput == nil {
		return
	}

	// TODO: Figure out why this is needed
	// o.audioOutput.SendAudio([]byte{})

	if !o.IsSpeaking || (o.turns.activeTurn() != nil && o.turns.activeTurn().Cancelled) {
		o.audioOutput.ClearBuffer()
		return
	}

}

// TODO: Calculate the error factor based on marks' timestamps
const errorFactor = 0.5

// TODO: Optimize memory at some point, it is not a great idea to just append
// to a slice when we already consumed a part of it. But it needs to be synced
// properly, probably a ring buffer makes sense.

type audioBuffer struct {
	sampleRate int

	chunksDone bool

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
			if b.chunksDone && b.audioPlayed == len(b.audio) {
				b.audioDone = true
				b.audioSignal.Broadcast()
			}
			break
		}
	}
}

func (b *audioBuffer) ChunksDone() {
	b.chunksDone = true
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
	// TODO: This should probably be locked
	b.chunksDone = true
	b.paused = false
	b.audio = [][]byte{}
	b.audioConsumed = 0
	b.audioDone = true
	b.audioSignal.Broadcast()
	b.audioDone = false
	b.audioMarks = []struct {
		name     string
		position int
	}{}
	b.audioMarksConsumed = 0
	b.audioPlayed = 0
	b.audioPlayingStarted = time.Time{}
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
