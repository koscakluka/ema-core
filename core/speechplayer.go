package orchestration

import (
	"strings"
	"sync"
	"time"

	"github.com/koscakluka/ema-core/core/audio"
	events "github.com/koscakluka/ema-core/core/events"
)

const minSpokenTextUpdateInterval = 10 * time.Millisecond
const maxSpokenTextUpdateInterval = 250 * time.Millisecond

type speechPlayer struct {
	mu sync.RWMutex

	textBuffer  *textBuffer
	audioBuffer *audioBuffer
	text        []string
	playedMarks int

	lastEmittedSpokenText       string
	hasEmittedSpokenText        bool
	lastEmittedPlaybackPlayhead int

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
		p.lastEmittedSpokenText = ""
		p.hasEmittedSpokenText = false
		p.lastEmittedPlaybackPlayhead = 0
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
	confirmed := false
	p.withAudioBuffer(func(audioBuffer *audioBuffer) {
		confirmed = audioBuffer.ConfirmMark(id)
	})
	if !confirmed {
		return nil
	}

	transcript := p.confirmTextMark()
	p.emitPlaybackProgress()
	if transcript != nil {
		p.emitEvent(events.NewAssistantPlaybackMarkPlayed(id, *transcript))
	}
	return transcript
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

	nextUpdate := p.emitPlaybackProgress()
	timer := time.NewTimer(clampSpokenTextUpdateInterval(nextUpdate))
	defer timer.Stop()

	for {
		select {
		case <-done:
			return
		case <-timer.C:
			nextUpdate = p.emitPlaybackProgress()
			timer.Reset(clampSpokenTextUpdateInterval(nextUpdate))
		}
	}
}

func (p *speechPlayer) emitPlaybackProgress() time.Duration {
	if p == nil {
		return defaultApproximateUpdateDelay
	}

	var spokenText string
	var spokenDelta string
	emitSpokenText := false
	var frame []byte
	nextUpdate := defaultApproximateUpdateDelay
	p.lockFor(func() {
		if p.audioBuffer == nil {
			return
		}

		progress, delta, approxPlayhead, updateDelay := p.audioBuffer.ApproximateProgressAndPlaybackDelta(p.lastEmittedPlaybackPlayhead)
		nextUpdate = updateDelay
		if approxPlayhead > p.lastEmittedPlaybackPlayhead {
			p.lastEmittedPlaybackPlayhead = approxPlayhead
		}
		if len(delta) > 0 {
			frame = delta
		}

		spokenText, spokenDelta, emitSpokenText = p.nextSpokenTextUpdateLocked(progress)
	})

	if emitSpokenText {
		p.emitEvent(events.NewAssistantPlaybackTranscriptUpdated(spokenText))
		p.emitEvent(events.NewAssistantPlaybackTranscriptSegment(spokenDelta))
	}

	if len(frame) > 0 {
		p.emitEvent(events.NewAssistantPlaybackFrame(frame))
	}

	return nextUpdate
}

func (p *speechPlayer) nextSpokenTextUpdateLocked(currentSegmentProgress float64) (string, string, bool) {
	spokenText := p.approximateSpokenTextSoFarLocked(currentSegmentProgress)

	previousSpokenText := p.lastEmittedSpokenText
	hasPreviousEmission := p.hasEmittedSpokenText
	if hasPreviousEmission && spokenText == previousSpokenText {
		return "", "", false
	}
	if !hasPreviousEmission && spokenText == "" {
		return "", "", false
	}
	if hasPreviousEmission && !strings.HasPrefix(spokenText, previousSpokenText) {
		return "", "", false
	}

	p.lastEmittedSpokenText = spokenText
	p.hasEmittedSpokenText = true

	spokenDelta := spokenText
	if hasPreviousEmission {
		spokenDelta = spokenText[len(previousSpokenText):]
	}

	return spokenText, spokenDelta, true
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

func (p *speechPlayer) confirmTextMark() *string {
	if p == nil {
		return nil
	}

	var transcript *string
	p.lockFor(func() {
		if p.playedMarks >= len(p.text) {
			return
		}

		segment := p.text[p.playedMarks]
		transcript = &segment
		p.playedMarks++
	})

	return transcript
}

func (p *speechPlayer) SpokenTextSoFar() string {
	var s string
	p.rLockFor(func() {
		if p.playedMarks <= 0 || len(p.text) == 0 {
			s = ""
			return
		}

		maxSegments := p.playedMarks
		if maxSegments > len(p.text) {
			maxSegments = len(p.text)
		}

		var spoken strings.Builder
		for i := 0; i < maxSegments; i++ {
			spoken.WriteString(p.text[i])
		}

		s = spoken.String()
	})
	return s

}
func (p *speechPlayer) approximateSpokenTextSoFarLocked(currentSegmentProgress float64) string {
	if p == nil {
		return ""
	}

	if currentSegmentProgress < 0 {
		currentSegmentProgress = 0
	} else if currentSegmentProgress > 1 {
		currentSegmentProgress = 1
	}

	maxSegments := p.playedMarks
	if maxSegments > len(p.text) {
		maxSegments = len(p.text)
	}

	var spoken strings.Builder
	for i := 0; i < maxSegments; i++ {
		spoken.WriteString(p.text[i])
	}

	if currentSegmentProgress == 0 || maxSegments >= len(p.text) {
		return spoken.String()
	}

	currentSegmentRunes := []rune(p.text[maxSegments])
	currentSegmentLen := len(currentSegmentRunes)
	if currentSegmentLen == 0 {
		return spoken.String()
	}

	charsToShow := int(float64(currentSegmentLen) * currentSegmentProgress)
	if charsToShow > currentSegmentLen {
		charsToShow = currentSegmentLen
	}

	spoken.WriteString(string(currentSegmentRunes[:charsToShow]))
	return spoken.String()
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

func clampSpokenTextUpdateInterval(interval time.Duration) time.Duration {
	if interval < minSpokenTextUpdateInterval {
		return minSpokenTextUpdateInterval
	}
	if interval > maxSpokenTextUpdateInterval {
		return maxSpokenTextUpdateInterval
	}
	return interval
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
