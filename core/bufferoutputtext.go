package orchestration

import (
	"strings"
	"sync"
)

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
