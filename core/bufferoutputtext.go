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
	textComplete   bool
	updateSignal   *sync.Cond
	cleared        bool
}

func newTextBuffer() *textBuffer {
	return &textBuffer{
		updateSignal: sync.NewCond(&sync.Mutex{}),
	}
}

func (b *textBuffer) AddChunk(chunk string) {
	b.chunks = append(b.chunks, chunk)
	b.updateSignal.Broadcast()
}

func (b *textBuffer) TextComplete() {
	b.textComplete = true
	b.updateSignal.Broadcast()
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
		if b.textComplete && b.chunksConsumed == len(b.chunks) {
			return
		}
		b.updateSignal.L.Lock()
		b.updateSignal.Wait()
		b.updateSignal.L.Unlock()
		if b.cleared {
			return
		}
	}
}

func (b *textBuffer) String() string {
	return strings.Join(b.chunks, "")
}

func (b *textBuffer) Clear() {
	b.cleared = true
	b.updateSignal.Broadcast()
}
