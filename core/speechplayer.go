package orchestration

import (
	"cmp"
	"strings"
	"sync"
	"time"

	"github.com/koscakluka/ema-core/core/audio"
	events "github.com/koscakluka/ema-core/core/events"
	"github.com/koscakluka/ema-core/internal/utils"
)

const defaultSpokenTimerDuration = 50 * time.Millisecond

type speechPlayer struct {
	mu sync.RWMutex

	textBuffer  *textBuffer
	audioBuffer *audioBuffer
	text        []string
	playedMarks int

	confirmedSpokenText strings.Builder
	currentMarkProgress float64
	bufferedMarkText    *string

	segmentationBoundaries string
	emitEvent              eventEmitter
}

func newSpeechPlayer() *speechPlayer {
	return &speechPlayer{
		textBuffer: newTextBuffer(),
		emitEvent:  noopEventEmitter,
	}
}

func (p *speechPlayer) InitBuffers(encodingInfo audio.EncodingInfo, segmentationBoundaries string) {
	p.lockFor(func() {
		p.textBuffer = newTextBuffer()
		p.audioBuffer = newAudioBuffer(encodingInfo)
		p.text = nil
		p.playedMarks = 0
		p.confirmedSpokenText = strings.Builder{}
		p.segmentationBoundaries = segmentationBoundaries
	})
}

func (p *speechPlayer) AddTextChunk(chunk string) {
	if chunk != "" {
		p.withTextBuffer(func(textBuffer *textBuffer) { textBuffer.AddChunk(chunk) })
	}
}

func (p *speechPlayer) TextOrMarks(yield func(textOrMark) bool) {
	var textBuffer *textBuffer
	var segmentationBoundaries string
	p.rLockFor(func() {
		textBuffer = p.textBuffer
		segmentationBoundaries = p.segmentationBoundaries
	})

	if textBuffer != nil {
		textBuffer.Chunks(func(chunk string) bool {
			if !yield(textOrMark{Type: textOrMarkTypeText, Text: chunk}) {
				return false
			}

			if chunk != "" {
				// add text
				p.lockFor(func() {
					if len(p.text) == 0 {
						p.text = append(p.text, "")
					}
					p.text[len(p.text)-1] += chunk
				})
			}
			if segmentationBoundaries == "" || !strings.ContainsAny(chunk, segmentationBoundaries) {
				return true
			}

			// mark
			p.lockFor(func() { p.text = append(p.text, "") })
			return yield(textOrMark{Type: textOrMarkTypeMark})
		})
		if segmentationBoundaries == "" {
			return
		}

		// mark
		// TODO: Check if this is actually necessary, we already
		// send a mark at the end of the text buffer.
		p.lockFor(func() { p.text = append(p.text, "") })
		if !yield(textOrMark{Type: textOrMarkTypeMark}) {
			return
		}
	}
}

func (p *speechPlayer) TextComplete() {
	p.withTextBuffer(func(textBuffer *textBuffer) { textBuffer.TextComplete() })
}

func (p *speechPlayer) ClearText() {
	p.withTextBuffer(func(textBuffer *textBuffer) { textBuffer.Clear() })
}

func (p *speechPlayer) FullText() (text string) {
	p.withTextBuffer(func(textBuffer *textBuffer) { text = textBuffer.String() })
	return text
}

func (p *speechPlayer) AddAudio(audio []byte) {
	p.withAudioBuffer(func(audioBuffer *audioBuffer) { audioBuffer.AddAudio(audio) })
}

// AddMark forwards a generated TTS mark to the audio buffer.
//
// Optional terminal=true marks explicit end-of-stream in legacy mode.
func (p *speechPlayer) AddMark(isTerminal ...bool) {
	terminal := len(isTerminal) > 0 && isTerminal[0]
	p.withAudioBuffer(func(audioBuffer *audioBuffer) { audioBuffer.Mark(terminal) })
}
func (p *speechPlayer) FinishAudio() {
	p.withAudioBuffer(func(audioBuffer *audioBuffer) { audioBuffer.AllAudioLoaded() })
}
func (p *speechPlayer) EnableLegacyMode() {
	p.withAudioBuffer(func(audioBuffer *audioBuffer) { audioBuffer.SetUsingLegacyTTSMode() })
}

func (p *speechPlayer) Audio(yield func(audioOrMark) bool) {
	var audioBuffer *audioBuffer
	p.rLockFor(func() { audioBuffer = p.audioBuffer })

	if audioBuffer != nil {
		emitterDone := make(chan struct{})
		go p.runProgressEmitter(emitterDone)
		playbackStarted := false
		audioBuffer.Audio(func(item audioOrMark) bool {
			consumed := yield(item)
			if consumed && !playbackStarted {
				p.emitEvent(events.NewAssistantPlaybackStarted())
				playbackStarted = true
			}
			return consumed
		})
		close(emitterDone)
		p.emitPlaybackProgress()
	}

	p.emitEvent(events.NewAssistantPlaybackEnded(p.FullText()))
}

func (p *speechPlayer) ConfirmOutputMark(id string) *string {
	var markText *string
	var emitEvent bool
	p.lockFor(func() {
		if ok := p.audioBuffer.ConfirmMark(id); !ok {
			return
		}
		if p.playedMarks >= len(p.text) {
			return
		}

		emitEvent = true
		markText = utils.Ptr(p.text[p.playedMarks])
		p.confirmedSpokenText.WriteString(*markText)
		p.advanceToNextMarkLocked()
	})
	if !emitEvent {
		return markText
	}

	p.emitPlaybackProgress()
	if markText != nil {
		p.emitEvent(events.NewAssistantPlaybackMarkPlayed(id, *markText))
	}
	return markText
}

func (p *speechPlayer) PauseAudio() {
	p.withAudioBuffer(func(audioBuffer *audioBuffer) { audioBuffer.Pause() })
}

func (p *speechPlayer) ResumeAudio() {
	p.withAudioBuffer(func(audioBuffer *audioBuffer) { audioBuffer.Resume() })
}

func (p *speechPlayer) StopAudio() {
	p.withAudioBuffer(func(audioBuffer *audioBuffer) { audioBuffer.Stop() })
}

func (p *speechPlayer) StopAndUnblock() {
	p.withAudioBuffer(func(audioBuffer *audioBuffer) {
		audioBuffer.AddAudio([]byte{})
		audioBuffer.Stop()
	})
}

func (p *speechPlayer) runProgressEmitter(done <-chan struct{}) {
	if p == nil {
		return
	}

	p.emitPlaybackProgress()
	timer := time.NewTimer(defaultSpokenTimerDuration)
	defer timer.Stop()

	for {
		select {
		case <-done:
			return
		case <-timer.C:
			p.emitPlaybackProgress()
			timer.Reset(defaultSpokenTimerDuration)
		}
	}
}

func (p *speechPlayer) emitPlaybackProgress() {
	if p == nil {
		return
	}

	var spokenText string
	var spokenDelta string
	// emitSpokenText := false
	var frame []byte
	p.lockFor(func() {
		if p.audioBuffer == nil {
			return
		}
		if p.bufferedMarkText != nil {
			spokenDelta = *p.bufferedMarkText
			p.bufferedMarkText = nil
		}

		var progress float64
		progress, frame = p.audioBuffer.Progress(p.playedMarks)

		progress = clamp(progress, 0, 1)
		if p.currentMarkProgress < progress {
			if p.playedMarks < len(p.text) && progress > 0 {
				currentSegmentText := getPercentOf(p.text[p.playedMarks], progress)

				spokenDelta += currentSegmentText[len(getPercentOf(p.text[p.playedMarks], p.currentMarkProgress)):]
				spokenText = p.confirmedSpokenText.String() + currentSegmentText
			}
			p.currentMarkProgress = progress
		}
	})

	if spokenText != "" {
		p.emitEvent(events.NewAssistantPlaybackTranscriptUpdated(spokenText))
		p.emitEvent(events.NewAssistantPlaybackTranscriptSegment(spokenDelta))
	}

	if len(frame) > 0 {
		p.emitEvent(events.NewAssistantPlaybackFrame(frame))
	}
}

func (p *speechPlayer) Snapshot() *speechPlayer {
	if p == nil {
		return p
	}

	snapshot := newSpeechPlayer()
	snapshot.SetEventEmitter(p.emitEvent)
	return snapshot
}

func (p *speechPlayer) SetEventEmitter(emitEvent eventEmitter) {
	if p == nil {
		return
	}

	p.lockFor(func() {
		if emitEvent == nil {
			p.emitEvent = noopEventEmitter
			return
		}
		p.emitEvent = emitEvent
	})
}

func (p *speechPlayer) SpokenTextSoFar() string {
	var s string
	p.rLockFor(func() { s = p.confirmedSpokenText.String() })
	return s

}

func (p *speechPlayer) advanceToNextMarkLocked() {
	if p != nil && p.playedMarks < len(p.text) {
		// HACK: We should be able to get this text somewhat easier
		p.bufferedMarkText = utils.Ptr(p.text[p.playedMarks][len(getPercentOf(p.text[p.playedMarks], p.currentMarkProgress)):])
		p.playedMarks++
		p.currentMarkProgress = 0
	}
}

func (p *speechPlayer) withTextBuffer(f func(*textBuffer)) {
	var textBuffer *textBuffer
	p.rLockFor(func() {
		textBuffer = p.textBuffer
	})
	if textBuffer != nil {
		f(textBuffer)
	}
}

func (p *speechPlayer) withAudioBuffer(f func(*audioBuffer)) {
	var audioBuffer *audioBuffer
	p.rLockFor(func() {
		audioBuffer = p.audioBuffer
	})
	if audioBuffer != nil {
		f(audioBuffer)
	}
}

func (p *speechPlayer) lockFor(f func()) {
	if p != nil {
		p.mu.Lock()
		defer p.mu.Unlock()
		f()
	}

}

func (p *speechPlayer) rLockFor(f func()) {
	if p != nil {
		p.mu.RLock()
		defer p.mu.RUnlock()
		f()
	}
}

type textOrMark struct {
	Type textOrMarkType
	Text string
}

type textOrMarkType string

const (
	textOrMarkTypeText textOrMarkType = "text"
	textOrMarkTypeMark textOrMarkType = "mark"
)

func getPercentOf(text string, percent float64) string {
	// TODO: Check if it makes more sense to return an error in this case or
	// even just return empty string (<=0) and full text (>=1).
	percent = clamp(percent, 0, 1)
	approxLen := int(float64(len(text)) * percent)
	return text[:min(approxLen, len(text))]
}

func clamp[T cmp.Ordered](x T, minValue T, maxValue T) T {
	return max(min(x, maxValue), minValue)
}
