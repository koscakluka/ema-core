package orchestration

import (
	"strings"
	"sync"
)

// TODO: Optimize memory at some point, it is not a great idea to just append
// to a slice when we already consumed a part of it. But it needs to be synced
// properly, probably a ring buffer makes sense.

type textBuffer struct {
	mu             sync.Mutex
	chunks         []string
	chunksConsumed int
	textComplete   bool
	updateSignal   chan struct{}
	cleared        bool
}

func newTextBuffer() *textBuffer {
	b := &textBuffer{
		updateSignal: make(chan struct{}, 1),
	}
	return b
}

func (b *textBuffer) AddChunk(chunk string) {
	b.mu.Lock()
	b.chunks = append(b.chunks, chunk)
	b.mu.Unlock()
	b.signalUpdate()
}

func (b *textBuffer) TextComplete() {
	b.mu.Lock()
	b.textComplete = true
	b.mu.Unlock()
	b.signalUpdate()
}

func (b *textBuffer) Chunks(yield func(string) bool) {
	for {
		b.mu.Lock()
		if b.cleared {
			b.mu.Unlock()
			return
		}

		if b.chunksConsumed < len(b.chunks) {
			chunk := b.chunks[b.chunksConsumed]
			b.chunksConsumed++
			b.mu.Unlock()
			if !yield(chunk) {
				return
			}
			continue
		}

		if b.textComplete {
			b.mu.Unlock()
			return
		}

		b.mu.Unlock()
		<-b.updateSignal
	}
}

func (b *textBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return strings.Join(b.chunks, "")
}

func (b *textBuffer) Clear() {
	b.mu.Lock()
	b.cleared = true
	b.mu.Unlock()
	b.signalUpdate()
}

func (b *textBuffer) signalUpdate() {
	select {
	case b.updateSignal <- struct{}{}:
	default:
	}
}
