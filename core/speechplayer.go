package orchestration

import (
	"strings"
	"sync"
	"time"

	"github.com/koscakluka/ema-core/core/audio"
)

const minSpokenTextUpdateInterval = 10 * time.Millisecond
const maxSpokenTextUpdateInterval = 250 * time.Millisecond

type speechPlayer struct {
	mu sync.RWMutex

	onAudioEnded      func(string)
	onSpokenText      func(string)
	onSpokenTextDelta func(string)
	textBuffer        *textBuffer
	audioBuffer       *audioBuffer
	text              []string
	playedMarks       int

	lastEmittedSpokenText string
	hasEmittedSpokenText  bool

	segmentationBoundaries string
}

func newSpeechPlayer() *speechPlayer {
	return &speechPlayer{
		onAudioEnded:      func(string) {},
		onSpokenText:      func(string) {},
		onSpokenTextDelta: func(string) {},
		textBuffer:        newTextBuffer(),
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
		p.segmentationBoundaries = segmentationBoundaries
	})
}

func (p *speechPlayer) AddTextChunk(chunk string) {
	if chunk != "" {
		p.lockFor(func() {
			if p.textBuffer != nil {
				p.textBuffer.AddChunk(chunk)
			}
		})
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
	p.rLockFor(func() {
		if p.textBuffer != nil {
			p.textBuffer.TextComplete()
		}
	})
}

func (p *speechPlayer) ClearText() {
	p.rLockFor(func() {
		if p.textBuffer != nil {
			p.textBuffer.Clear()
		}
	})
}

func (p *speechPlayer) FullText() string {
	var text string
	p.rLockFor(func() {
		if p.textBuffer != nil {
			text = p.textBuffer.String()
		}
	})
	return text
}

func (p *speechPlayer) AddAudioChunk(audio []byte) {
	p.rLockFor(func() {
		if p.audioBuffer != nil {
			p.audioBuffer.AddAudio(audio)
		}
	})
}

func (p *speechPlayer) AddAudioMark(transcript string) {
	p.rLockFor(func() {
		if p.audioBuffer != nil {
			p.audioBuffer.Mark(transcript)
		}
	})
}

func (p *speechPlayer) AllAudioLoaded() {
	p.rLockFor(func() {
		if p.audioBuffer != nil {
			p.audioBuffer.AllAudioLoaded()
		}
	})
}

func (p *speechPlayer) EnableLegacyTTSMode() {
	p.rLockFor(func() {
		if p.audioBuffer != nil {
			p.audioBuffer.SetUsingLegacyTTSMode()
		}
	})
}

func (p *speechPlayer) Audio(yield func(audioOrMark) bool) {
	var audioBuffer *audioBuffer
	p.rLockFor(func() { audioBuffer = p.audioBuffer })

	if audioBuffer != nil {
		emitterDone := make(chan struct{})
		go p.startApproximateSpokenTextEmitter(emitterDone)
		audioBuffer.Audio(yield)
		close(emitterDone)
	}

	p.OnAudioEnded(p.FullText())
}

func (p *speechPlayer) OnAudioOutputMarkPlayed(id string) *string {
	var transcript *string
	p.lockFor(func() {
		if p.audioBuffer != nil {
			transcript = p.audioBuffer.GetMarkText(id)
			p.audioBuffer.ConfirmMark(id)
		}
	})
	p.ConfirmMark()
	p.EmitApproximateSpokenText(p.ApproximateCurrentSegmentProgress())
	return transcript
}

func (p *speechPlayer) ApproximateCurrentSegmentProgress() float64 {
	var progress float64
	p.rLockFor(func() {
		if p.audioBuffer != nil {
			progress = p.audioBuffer.ApproximateCurrentSegmentProgress()
		}
	})
	return progress
}

func (p *speechPlayer) ApproximateCurrentSegmentProgressAndNextUpdate() (float64, time.Duration) {
	progress, nextUpdate := 0.0, defaultApproximateUpdateDelay
	p.rLockFor(func() {
		if p.audioBuffer != nil {
			progress, nextUpdate = p.audioBuffer.ApproximateCurrentSegmentProgressAndNextUpdate()
		}
	})
	return progress, nextUpdate
}

func (p *speechPlayer) PauseAudio() {
	p.rLockFor(func() {
		if p.audioBuffer != nil {
			p.audioBuffer.Pause()
		}
	})
}

func (p *speechPlayer) ResumeAudio() {
	p.rLockFor(func() {
		if p.audioBuffer != nil {
			p.audioBuffer.Resume()
		}
	})
}

func (p *speechPlayer) StopAudio() {
	p.rLockFor(func() {
		if p.audioBuffer != nil {
			p.audioBuffer.Stop()
		}
	})
}

func (p *speechPlayer) StopAudioAndUnblock() {
	p.rLockFor(func() {
		if p.audioBuffer != nil {
			p.audioBuffer.AddAudio([]byte{})
			p.audioBuffer.Stop()
		}
	})
}

func (p *speechPlayer) EmitApproximateSpokenTextFromAudioProgress() {
	p.EmitApproximateSpokenText(p.ApproximateCurrentSegmentProgress())
}

func (p *speechPlayer) EmitApproximateSpokenTextFromAudioProgressAndNextUpdate() time.Duration {
	progress, nextUpdate := p.ApproximateCurrentSegmentProgressAndNextUpdate()
	p.EmitApproximateSpokenText(progress)
	return nextUpdate
}

func (p *speechPlayer) startApproximateSpokenTextEmitter(done <-chan struct{}) {
	if p != nil {
		progress, nextUpdate := p.ApproximateCurrentSegmentProgressAndNextUpdate()
		if p.ApproximateSpokenTextSoFar(progress) != "" {
			p.EmitApproximateSpokenText(progress)
		}

		timer := time.NewTimer(clampSpokenTextUpdateInterval(nextUpdate))
		defer timer.Stop()
		for {
			select {
			case <-done:
				return
			case <-timer.C:
				nextUpdate = p.EmitApproximateSpokenTextFromAudioProgressAndNextUpdate()
				timer.Reset(clampSpokenTextUpdateInterval(nextUpdate))
			}
		}
	}
}

func (p *speechPlayer) Snapshot() *speechPlayer {
	if p == nil {
		return p
	}

	p.mu.RLock()
	onAudioEnded := p.onAudioEnded
	onSpokenText := p.onSpokenText
	onSpokenTextDelta := p.onSpokenTextDelta
	p.mu.RUnlock()

	snapshot := newSpeechPlayer()
	snapshot.SetCallbacks(onAudioEnded)
	snapshot.SetSpokenTextCallback(onSpokenText)
	snapshot.SetSpokenTextDeltaCallback(onSpokenTextDelta)
	return snapshot
}

func (p *speechPlayer) SetCallbacks(onAudioEnded func(string)) {
	if onAudioEnded != nil {
		p.lockFor(func() { p.onAudioEnded = onAudioEnded })
	}
}

func (p *speechPlayer) SetSpokenTextCallback(onSpokenText func(string)) {
	if onSpokenText != nil {
		p.lockFor(func() {
			p.onSpokenText = onSpokenText
			p.hasEmittedSpokenText = false
			p.lastEmittedSpokenText = ""
		})
	}
}

func (p *speechPlayer) SetSpokenTextDeltaCallback(onSpokenTextDelta func(string)) {
	if onSpokenTextDelta != nil {
		p.lockFor(func() {
			p.onSpokenTextDelta = onSpokenTextDelta
			p.hasEmittedSpokenText = false
			p.lastEmittedSpokenText = ""
		})
	}
}

func (p *speechPlayer) ConfirmMark() {
	p.lockFor(func() {
		if p.playedMarks >= len(p.text) {
			return
		}
		p.playedMarks++
	})
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

func (p *speechPlayer) ApproximateSpokenTextSoFar(currentSegmentProgress float64) string {
	if p == nil {
		return ""
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

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

func (p *speechPlayer) EmitApproximateSpokenText(currentSegmentProgress float64) {
	if p == nil {
		return
	}

	spokenText := p.ApproximateSpokenTextSoFar(currentSegmentProgress)

	p.mu.Lock()
	onSpokenText := p.onSpokenText
	onSpokenTextDelta := p.onSpokenTextDelta
	previousSpokenText := p.lastEmittedSpokenText
	hasPreviousEmission := p.hasEmittedSpokenText
	if p.hasEmittedSpokenText && spokenText == p.lastEmittedSpokenText {
		p.mu.Unlock()
		return
	}
	p.lastEmittedSpokenText = spokenText
	p.hasEmittedSpokenText = true
	p.mu.Unlock()

	if onSpokenText != nil {
		onSpokenText(spokenText)
	}

	if onSpokenTextDelta != nil {
		delta := spokenText
		if hasPreviousEmission && strings.HasPrefix(spokenText, previousSpokenText) {
			delta = spokenText[len(previousSpokenText):]
		}
		onSpokenTextDelta(delta)
	}
}

func (p *speechPlayer) OnAudioEnded(transcript string) {
	var onAudioEnded func(string)
	p.rLockFor(func() { onAudioEnded = p.onAudioEnded })
	if onAudioEnded != nil {
		onAudioEnded(transcript)
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
