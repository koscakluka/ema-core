package orchestration

type speechPlayer struct {
	onAudioEnded func(string)
}

func newSpeechPlayer() *speechPlayer {
	return &speechPlayer{
		onAudioEnded: func(string) {},
	}
}

func (p *speechPlayer) Snapshot() *speechPlayer {
	if p == nil {
		return p
	}

	snapshot := newSpeechPlayer()
	snapshot.SetCallbacks(p.onAudioEnded)
	return snapshot
}

func (p *speechPlayer) SetCallbacks(onAudioEnded func(string)) {
	if p == nil {
		return
	}

	if onAudioEnded != nil {
		p.onAudioEnded = onAudioEnded
	}
}

func (p *speechPlayer) OnAudioEnded(transcript string) {
	if p == nil {
		return
	}

	if p.onAudioEnded != nil {
		p.onAudioEnded(transcript)
	}
}
