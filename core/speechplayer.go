package orchestration

import (
	"strings"
	"sync"
)

type speechPlayer struct {
	mu sync.RWMutex

	onAudioEnded      func(string)
	onSpokenText      func(string)
	onSpokenTextDelta func(string)
	text              []string
	playedMarks       int

	lastEmittedSpokenText string
	hasEmittedSpokenText  bool
}

func newSpeechPlayer() *speechPlayer {
	return &speechPlayer{
		onAudioEnded:      func(string) {},
		onSpokenText:      func(string) {},
		onSpokenTextDelta: func(string) {},
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
	if p == nil {
		return
	}

	if onAudioEnded != nil {
		p.mu.Lock()
		p.onAudioEnded = onAudioEnded
		p.mu.Unlock()
	}
}

func (p *speechPlayer) SetSpokenTextCallback(onSpokenText func(string)) {
	if p == nil {
		return
	}

	if onSpokenText != nil {
		p.mu.Lock()
		p.onSpokenText = onSpokenText
		p.hasEmittedSpokenText = false
		p.lastEmittedSpokenText = ""
		p.mu.Unlock()
	}
}

func (p *speechPlayer) SetSpokenTextDeltaCallback(onSpokenTextDelta func(string)) {
	if p == nil {
		return
	}

	if onSpokenTextDelta != nil {
		p.mu.Lock()
		p.onSpokenTextDelta = onSpokenTextDelta
		p.hasEmittedSpokenText = false
		p.lastEmittedSpokenText = ""
		p.mu.Unlock()
	}
}

func (p *speechPlayer) AddText(text string) {
	if p == nil || text == "" {
		return
	}

	p.mu.Lock()
	if len(p.text) == 0 {
		p.text = append(p.text, "")
	}
	p.text[len(p.text)-1] += text
	p.mu.Unlock()
}

func (p *speechPlayer) Mark() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.text = append(p.text, "")
}

func (p *speechPlayer) ConfirmMark() {
	if p == nil {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.playedMarks >= len(p.text) {
		return
	}
	p.playedMarks++
}

func (p *speechPlayer) SpokenTextSoFar() string {
	if p == nil {
		return ""
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.playedMarks <= 0 || len(p.text) == 0 {
		return ""
	}

	maxSegments := p.playedMarks
	if maxSegments > len(p.text) {
		maxSegments = len(p.text)
	}

	var spoken strings.Builder
	for i := 0; i < maxSegments; i++ {
		spoken.WriteString(p.text[i])
	}

	return spoken.String()
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
	if p == nil {
		return
	}

	p.mu.RLock()
	onAudioEnded := p.onAudioEnded
	p.mu.RUnlock()

	if onAudioEnded != nil {
		onAudioEnded(transcript)
	}
}
