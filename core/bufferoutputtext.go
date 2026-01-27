package orchestration

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/koscakluka/ema-core/core/llms"
)

func (o *Orchestrator) passTextToTTS(ctx context.Context) {
	ctx, span := tracer.Start(ctx, "passing text to tts")
	defer span.End()
textLoop:
	for chunk := range o.outputTextBuffer.Chunks {
		activeTurn := o.turns.activeTurn()
		if activeTurn != nil && activeTurn.Cancelled {
			break textLoop
		}
		if activeTurn != nil && activeTurn.Stage != llms.TurnStageSpeaking {
			activeTurn.Stage = llms.TurnStageSpeaking
			o.turns.updateActiveTurn(*activeTurn)
		}

		if o.orchestrateOptions.onResponse != nil {
			o.orchestrateOptions.onResponse(chunk)
		}
		if o.textToSpeechClient != nil {
			if err := o.textToSpeechClient.SendText(chunk); err != nil {
				log.Printf("Failed to send text to deepgram: %v", err)
			}
			if o.audioOutput != nil {
				if _, ok := o.audioOutput.(AudioOutputV1); ok {
					if strings.ContainsAny(chunk, ".?!") {
						if err := o.textToSpeechClient.FlushBuffer(); err != nil {
							log.Printf("Failed to flush buffer: %v", err)
						}
					}
				}
			}
		}
	}

	if o.textToSpeechClient != nil {
		if err := o.textToSpeechClient.FlushBuffer(); err != nil {
			log.Printf("Failed to flush buffer: %v", err)
		}
	} else if o.turns.activeTurn() != nil && !o.turns.activeTurn().Cancelled {
		o.finaliseActiveTurn()
		o.promptEnded.Done()

	}

	if o.orchestrateOptions.onResponseEnd != nil {
		o.orchestrateOptions.onResponseEnd()
	}

}

// TODO: Optimize memory at some point, it is not a great idea to just append
// to a slice when we already consumed a part of it. But it needs to be synced
// properly, probably a ring buffer makes sense.

type textBuffer struct {
	chunks         []string
	chunksConsumed int
	chunksDone     bool
	chunksSignal   *sync.Cond
}

func newTextBuffer() *textBuffer {
	return &textBuffer{
		chunksSignal: sync.NewCond(&sync.Mutex{}),
	}
}

func (b *textBuffer) AddChunk(chunk string) {
	b.chunks = append(b.chunks, chunk)
	b.chunksSignal.Broadcast()
}

func (b *textBuffer) ChunksDone() {
	b.chunksDone = true
	b.chunksSignal.Broadcast()
}

func (b *textBuffer) Chunks(yield func(string) bool) {
	for {
		for {
			if len(b.chunks) == b.chunksConsumed {
				break
			}
			chunk := b.chunks[b.chunksConsumed]
			b.chunksConsumed++
			if !yield(chunk) {
				return
			}
		}
		if b.chunksDone && b.chunksConsumed == len(b.chunks) {
			return
		}
		b.chunksSignal.L.Lock()
		b.chunksSignal.Wait()
		b.chunksSignal.L.Unlock()
	}
}

func (b *textBuffer) AllChunks() string {
	return strings.Join(b.chunks, "")
}

func (b *textBuffer) Clear() {
	// TODO: This should probably be locked
	b.chunks = []string{}
	b.chunksConsumed = 0
	b.chunksDone = true
	b.chunksSignal.Broadcast()
	b.chunksDone = false
}
