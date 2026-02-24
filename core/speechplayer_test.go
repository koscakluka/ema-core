package orchestration

import (
	"bytes"
	"testing"
	"time"

	"github.com/koscakluka/ema-core/core/audio"
	events "github.com/koscakluka/ema-core/core/events"
)

func setTextSegments(player *speechPlayer, segments ...string) {
	player.text = append([]string(nil), segments...)
}

func emitSpokenProgress(player *speechPlayer, progress float64) {
	spokenText, spokenDelta, emit := "", "", false
	player.lockFor(func() {
		spokenText, spokenDelta, emit = player.nextSpokenTextUpdateLocked(progress)
	})
	if !emit {
		return
	}

	player.emitEvent(events.NewAssistantPlaybackTranscriptUpdated(spokenText))
	player.emitEvent(events.NewAssistantPlaybackTranscriptSegment(spokenDelta))
}

func confirmSpokenMark(player *speechPlayer) {
	_ = player.confirmTextMark()
}

func approximateSpokenText(player *speechPlayer, progress float64) (spoken string) {
	player.rLockFor(func() {
		spoken = player.approximateSpokenTextSoFarLocked(progress)
	})
	return spoken
}

func emitPlaybackProgress(player *speechPlayer) {
	player.emitPlaybackProgress()
}

func TestSpeechPlayerAddTextTracksCurrentSegment(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello world")

	if len(player.text) != 1 {
		t.Fatalf("expected one text segment, got %d", len(player.text))
	}
	if got := player.text[0]; got != "Hello world" {
		t.Fatalf("expected segment text %q, got %q", "Hello world", got)
	}
}

func TestSpeechPlayerMarkStartsNewSegment(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello", " world")

	if len(player.text) != 2 {
		t.Fatalf("expected two text segments, got %d", len(player.text))
	}
	if got := player.text[0]; got != "Hello" {
		t.Fatalf("expected first segment %q, got %q", "Hello", got)
	}
	if got := player.text[1]; got != " world" {
		t.Fatalf("expected second segment %q, got %q", " world", got)
	}
}

func TestSpeechPlayerSpokenTextSoFarFollowsConfirmedMarks(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello", " world", "!")

	if got := player.SpokenTextSoFar(); got != "" {
		t.Fatalf("expected no spoken text before marks are confirmed, got %q", got)
	}

	confirmSpokenMark(player)
	if got := player.SpokenTextSoFar(); got != "Hello" {
		t.Fatalf("expected spoken text %q after first mark, got %q", "Hello", got)
	}

	confirmSpokenMark(player)
	if got := player.SpokenTextSoFar(); got != "Hello world" {
		t.Fatalf("expected spoken text %q after second mark, got %q", "Hello world", got)
	}
}

func TestSpeechPlayerConfirmMarkDoesNotOverrun(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello", "")
	confirmSpokenMark(player)
	confirmSpokenMark(player)
	confirmSpokenMark(player)

	if got := player.SpokenTextSoFar(); got != "Hello" {
		t.Fatalf("expected spoken text to remain %q when over-confirmed, got %q", "Hello", got)
	}
}

func TestSpeechPlayerApproximateSpokenTextSoFarIncludesCurrentSegment(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello", " world")

	if got := approximateSpokenText(player, 0.5); got != "He" {
		t.Fatalf("expected approximate spoken text %q, got %q", "He", got)
	}

	confirmSpokenMark(player)
	if got := approximateSpokenText(player, 0.5); got != "Hello wo" {
		t.Fatalf("expected approximate spoken text %q, got %q", "Hello wo", got)
	}
}

func TestSpeechPlayerApproximateSpokenTextSoFarClampsProgress(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello", " world")
	confirmSpokenMark(player)

	if got := approximateSpokenText(player, -1); got != "Hello" {
		t.Fatalf("expected clamped lower bound result %q, got %q", "Hello", got)
	}
	if got := approximateSpokenText(player, 2); got != "Hello world" {
		t.Fatalf("expected clamped upper bound result %q, got %q", "Hello world", got)
	}
}

func TestSpeechPlayerEmitApproximateSpokenTextEmitsEvent(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello", " world")

	updates := []string{}
	player.SetEventEmitter(func(event events.Event) {
		if spokenText, ok := event.(events.AssistantPlaybackTranscriptUpdated); ok {
			updates = append(updates, spokenText.Transcript)
		}
	})

	emitSpokenProgress(player, 0.5)
	confirmSpokenMark(player)
	emitSpokenProgress(player, 0.5)

	if len(updates) != 2 {
		t.Fatalf("expected 2 spoken text updates, got %d", len(updates))
	}
	if updates[0] != "He" {
		t.Fatalf("expected first update %q, got %q", "He", updates[0])
	}
	if updates[1] != "Hello wo" {
		t.Fatalf("expected second update %q, got %q", "Hello wo", updates[1])
	}
}

func TestSpeechPlayerEmitApproximateSpokenTextSkipsUnchangedValues(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello")

	updates := []string{}
	player.SetEventEmitter(func(event events.Event) {
		if spokenText, ok := event.(events.AssistantPlaybackTranscriptUpdated); ok {
			updates = append(updates, spokenText.Transcript)
		}
	})

	emitSpokenProgress(player, 0.5)
	emitSpokenProgress(player, 0.5)
	emitSpokenProgress(player, 0.5)

	if len(updates) != 1 {
		t.Fatalf("expected 1 spoken text update for unchanged value, got %d", len(updates))
	}
	if updates[0] != "He" {
		t.Fatalf("expected update %q, got %q", "He", updates[0])
	}
}

func TestSpeechPlayerEmitApproximateSpokenTextDeltaReportsIncrementalChange(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello")

	deltas := []string{}
	player.SetEventEmitter(func(event events.Event) {
		if spokenTextDelta, ok := event.(events.AssistantPlaybackTranscriptSegment); ok {
			deltas = append(deltas, spokenTextDelta.Segment)
		}
	})

	emitSpokenProgress(player, 0.2)
	emitSpokenProgress(player, 0.6)

	if len(deltas) != 2 {
		t.Fatalf("expected 2 spoken text deltas, got %d", len(deltas))
	}
	if deltas[0] != "H" {
		t.Fatalf("expected first delta %q, got %q", "H", deltas[0])
	}
	if deltas[1] != "el" {
		t.Fatalf("expected second delta %q, got %q", "el", deltas[1])
	}
}

func TestSpeechPlayerEmitApproximateSpokenTextDeltaSkipsRegression(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello")

	deltas := []string{}
	player.SetEventEmitter(func(event events.Event) {
		if spokenTextDelta, ok := event.(events.AssistantPlaybackTranscriptSegment); ok {
			deltas = append(deltas, spokenTextDelta.Segment)
		}
	})

	emitSpokenProgress(player, 1)
	emitSpokenProgress(player, 0.2)

	if len(deltas) != 1 {
		t.Fatalf("expected 1 spoken text delta when playback progress regresses, got %d", len(deltas))
	}
	if deltas[0] != "Hello" {
		t.Fatalf("expected first delta %q, got %q", "Hello", deltas[0])
	}
}

func TestSpeechPlayerOnAudioEndedEmitsProvidedTranscript(t *testing.T) {
	player := newSpeechPlayer()

	transcripts := []string{}
	player.SetEventEmitter(func(event events.Event) {
		if audioEnded, ok := event.(events.AssistantPlaybackEnded); ok {
			transcripts = append(transcripts, audioEnded.Transcript)
		}
	})

	player.emitEvent(events.NewAssistantPlaybackEnded("full generated transcript"))

	if len(transcripts) != 1 || transcripts[0] != "full generated transcript" {
		t.Fatalf("expected one audio-ended transcript %q, got %v", "full generated transcript", transcripts)
	}
}

func TestSpeechPlayerSnapshotKeepsEmitterButNotMarkedText(t *testing.T) {
	player := newSpeechPlayer()
	setTextSegments(player, "already queued")

	audioEnded := []string{}
	spoken := []string{}
	spokenDeltas := []string{}
	player.SetEventEmitter(func(event events.Event) {
		switch typedEvent := event.(type) {
		case events.AssistantPlaybackEnded:
			audioEnded = append(audioEnded, typedEvent.Transcript)
		case events.AssistantPlaybackTranscriptUpdated:
			spoken = append(spoken, typedEvent.Transcript)
		case events.AssistantPlaybackTranscriptSegment:
			spokenDeltas = append(spokenDeltas, typedEvent.Segment)
		}
	})

	snapshot := player.Snapshot()
	if len(snapshot.text) != 0 {
		t.Fatalf("expected snapshot text queue to be empty, got %d segments", len(snapshot.text))
	}

	snapshot.emitEvent(events.NewAssistantPlaybackEnded("new turn transcript"))
	if len(audioEnded) != 1 || audioEnded[0] != "new turn transcript" {
		t.Fatalf("expected snapshot audio-ended transcript %q, got %v", "new turn transcript", audioEnded)
	}

	setTextSegments(snapshot, "Hello")
	emitSpokenProgress(snapshot, 1)
	if len(spoken) != 1 || spoken[0] != "Hello" {
		t.Fatalf("expected snapshot spoken-text event %q, got %v", "Hello", spoken)
	}
	if len(spokenDeltas) != 1 || spokenDeltas[0] != "Hello" {
		t.Fatalf("expected snapshot spoken-text delta event %q, got %v", "Hello", spokenDeltas)
	}
}

func TestSpeechPlayerTextBufferOwnership(t *testing.T) {
	player := newSpeechPlayer()
	player.InitBuffers(audio.GetDefaultEncodingInfo(), "")

	player.AddTextChunk("Hello")
	player.AddTextChunk(" world")
	player.TextComplete()

	chunks := []string{}
	marks := 0
	for item := range player.TextOrMarks {
		switch item.Type {
		case textOrMarkTypeText:
			chunks = append(chunks, item.Text)
		case textOrMarkTypeMark:
			marks++
		}
	}

	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks in owned text buffer, got %d", len(chunks))
	}
	if chunks[0] != "Hello" {
		t.Fatalf("expected first chunk %q, got %q", "Hello", chunks[0])
	}
	if chunks[1] != " world" {
		t.Fatalf("expected second chunk %q, got %q", " world", chunks[1])
	}
	if marks != 0 {
		t.Fatalf("expected no marks with boundaries disabled, got %d", marks)
	}
	if got := player.FullText(); got != "Hello world" {
		t.Fatalf("expected full owned text %q, got %q", "Hello world", got)
	}
}

func TestSpeechPlayerOnAudioOutputMarkPlayedReturnsTranscript(t *testing.T) {
	player := newSpeechPlayer()
	player.InitBuffers(audio.GetDefaultEncodingInfo(), "")
	setTextSegments(player, "Hello")

	player.AddAudio([]byte{1, 2, 3})
	player.AddMark()

	markID := ""
	for audioOrMark := range player.Audio {
		if audioOrMark.Type == "mark" {
			markID = audioOrMark.Mark
			break
		}
	}

	if markID == "" {
		t.Fatalf("expected owned audio buffer to emit a mark")
	}

	transcript := player.ConfirmOutputMark(markID)
	if transcript == nil {
		t.Fatalf("expected transcript for confirmed mark")
	}
	if *transcript != "Hello" {
		t.Fatalf("expected transcript %q, got %q", "Hello", *transcript)
	}
}

func TestSpeechPlayerOnAudioOutputMarkPlayedCombinesConfirmationAndEmission(t *testing.T) {
	player := newSpeechPlayer()
	player.InitBuffers(audio.GetDefaultEncodingInfo(), "")

	setTextSegments(player, "Hello", " world")

	updates := []string{}
	markEvents := []events.AssistantPlaybackMarkPlayed{}
	player.SetEventEmitter(func(event events.Event) {
		switch typedEvent := event.(type) {
		case events.AssistantPlaybackTranscriptUpdated:
			updates = append(updates, typedEvent.Transcript)
		case events.AssistantPlaybackMarkPlayed:
			markEvents = append(markEvents, typedEvent)
		}
	})

	player.AddAudio([]byte{1, 2, 3})
	player.AddMark()

	markID := ""
	for audioOrMark := range player.Audio {
		if audioOrMark.Type == "mark" {
			markID = audioOrMark.Mark
			break
		}
	}

	if markID == "" {
		t.Fatalf("expected owned audio buffer to emit a mark")
	}

	transcript := player.ConfirmOutputMark(markID)
	if transcript == nil || *transcript != "Hello" {
		t.Fatalf("expected combined mark handling transcript %q, got %v", "Hello", transcript)
	}

	if got := player.SpokenTextSoFar(); got != "Hello" {
		t.Fatalf("expected spoken text to advance to %q, got %q", "Hello", got)
	}
	if len(updates) != 1 || updates[0] != "Hello" {
		t.Fatalf("expected one spoken-text emission %q, got %v", "Hello", updates)
	}
	if len(markEvents) != 1 {
		t.Fatalf("expected one playback mark played event, got %d", len(markEvents))
	}
	if markEvents[0].Mark != markID {
		t.Fatalf("expected playback mark played event mark %q, got %q", markID, markEvents[0].Mark)
	}
	if markEvents[0].Transcript != "Hello" {
		t.Fatalf("expected playback mark played event transcript %q, got %q", "Hello", markEvents[0].Transcript)
	}
}

func TestSpeechPlayerOnAudioOutputMarkPlayedIgnoresUnknownOrDuplicateMarks(t *testing.T) {
	player := newSpeechPlayer()
	player.InitBuffers(audio.GetDefaultEncodingInfo(), "")

	setTextSegments(player, "Hello", " world")

	updates := []string{}
	player.SetEventEmitter(func(event events.Event) {
		if spokenText, ok := event.(events.AssistantPlaybackTranscriptUpdated); ok {
			updates = append(updates, spokenText.Transcript)
		}
	})

	player.AddAudio([]byte{1, 2, 3})
	player.AddMark()

	markID := ""
	for audioOrMark := range player.Audio {
		if audioOrMark.Type == "mark" {
			markID = audioOrMark.Mark
			break
		}
	}

	if markID == "" {
		t.Fatalf("expected owned audio buffer to emit a mark")
	}

	if transcript := player.ConfirmOutputMark("unknown-mark"); transcript != nil {
		t.Fatalf("expected unknown mark to return nil transcript, got %q", *transcript)
	}
	if got := player.SpokenTextSoFar(); got != "" {
		t.Fatalf("expected unknown mark to not advance spoken text, got %q", got)
	}

	first := player.ConfirmOutputMark(markID)
	if first == nil || *first != "Hello" {
		t.Fatalf("expected first confirmation transcript %q, got %v", "Hello", first)
	}

	second := player.ConfirmOutputMark(markID)
	if second != nil {
		t.Fatalf("expected duplicate mark callback to return nil transcript, got %q", *second)
	}

	if got := player.SpokenTextSoFar(); got != "Hello" {
		t.Fatalf("expected duplicate mark to not advance spoken text beyond %q, got %q", "Hello", got)
	}
	if len(updates) != 1 || updates[0] != "Hello" {
		t.Fatalf("expected exactly one spoken-text emission %q, got %v", "Hello", updates)
	}
}

func TestSpeechPlayerAudioEmitsPlaybackStartedWhenAudioIsConsumed(t *testing.T) {
	player := newSpeechPlayer()
	player.InitBuffers(audio.GetDefaultEncodingInfo(), "")

	started := 0
	player.SetEventEmitter(func(event events.Event) {
		if _, ok := event.(events.AssistantPlaybackStarted); ok {
			started++
		}
	})

	player.AddAudio([]byte{1, 2, 3})
	player.AddMark()

	player.Audio(func(item audioOrMark) bool {
		return item.Type == audioOrMarkTypeAudio
	})

	if started != 1 {
		t.Fatalf("expected one playback started event, got %d", started)
	}
}

func TestSpeechPlayerAudioSkipsPlaybackStartedWhenFirstItemRejected(t *testing.T) {
	player := newSpeechPlayer()
	player.InitBuffers(audio.GetDefaultEncodingInfo(), "")

	started := 0
	player.SetEventEmitter(func(event events.Event) {
		if _, ok := event.(events.AssistantPlaybackStarted); ok {
			started++
		}
	})

	player.AddAudio([]byte{1, 2, 3})
	player.AddMark()

	player.Audio(func(item audioOrMark) bool {
		_ = item
		return false
	})

	if started != 0 {
		t.Fatalf("expected no playback started event when first item is rejected, got %d", started)
	}
}

func TestSpeechPlayerTextOrMarksEmitsBoundaryMarkWhenConfigured(t *testing.T) {
	player := newSpeechPlayer()
	player.InitBuffers(audio.GetDefaultEncodingInfo(), "?.!")

	player.AddTextChunk("Hello.")
	player.TextComplete()

	items := []textOrMark{}
	for item := range player.TextOrMarks {
		items = append(items, item)
	}

	if len(items) != 3 {
		t.Fatalf("expected one text and two mark events (boundary + trailing), got %d items", len(items))
	}
	if items[0].Type != textOrMarkTypeText || items[0].Text != "Hello." {
		t.Fatalf("expected first event to be text %q, got %#v", "Hello.", items[0])
	}
	if items[1].Type != textOrMarkTypeMark {
		t.Fatalf("expected second event to be mark, got %#v", items[1])
	}
	if items[2].Type != textOrMarkTypeMark {
		t.Fatalf("expected third event to be trailing mark, got %#v", items[2])
	}
	if len(player.text) != 3 {
		t.Fatalf("expected boundary and trailing segmentation to create two next segments, got %d", len(player.text))
	}
}

func TestSpeechPlayerTextOrMarksDoesNotEmitMarkWhenDisabled(t *testing.T) {
	player := newSpeechPlayer()
	player.InitBuffers(audio.GetDefaultEncodingInfo(), "")

	player.AddTextChunk("Hello.")
	player.TextComplete()

	items := []textOrMark{}
	for item := range player.TextOrMarks {
		items = append(items, item)
	}

	if len(items) != 1 {
		t.Fatalf("expected only one text event when boundaries disabled, got %d", len(items))
	}
	if items[0].Type != textOrMarkTypeText || items[0].Text != "Hello." {
		t.Fatalf("expected text event %q, got %#v", "Hello.", items[0])
	}
	if len(player.text) != 1 {
		t.Fatalf("expected no boundary segmentation when disabled, got %d segments", len(player.text))
	}
	if got := player.text[0]; got != "Hello." {
		t.Fatalf("expected text to remain in current segment %q, got %q", "Hello.", got)
	}
}

func TestSpeechPlayerTextOrMarksEmitsTrailingMarkWithoutBoundary(t *testing.T) {
	player := newSpeechPlayer()
	player.InitBuffers(audio.GetDefaultEncodingInfo(), "?.!")

	player.AddTextChunk("Hello world")
	player.TextComplete()

	items := []textOrMark{}
	for item := range player.TextOrMarks {
		items = append(items, item)
	}

	if len(items) != 2 {
		t.Fatalf("expected one text and one trailing mark event without boundary, got %d", len(items))
	}
	if items[0].Type != textOrMarkTypeText || items[0].Text != "Hello world" {
		t.Fatalf("expected text event %q, got %#v", "Hello world", items[0])
	}
	if items[1].Type != textOrMarkTypeMark {
		t.Fatalf("expected second event to be trailing mark, got %#v", items[1])
	}
	if len(player.text) != 2 {
		t.Fatalf("expected trailing segmentation without boundary, got %d segments", len(player.text))
	}
}

func TestSpeechPlayerEmitApproximatePlaybackFrameEmitsEvent(t *testing.T) {
	player := newSpeechPlayer()
	player.InitBuffers(audio.GetDefaultEncodingInfo(), "")

	frames := [][]byte{}
	player.SetEventEmitter(func(event events.Event) {
		if playbackFrame, ok := event.(events.AssistantPlaybackFrame); ok {
			frames = append(frames, append([]byte(nil), playbackFrame.Audio...))
		}
	})

	player.AddAudio([]byte{1, 2})
	player.AddAudio([]byte{3, 4})

	player.audioBuffer.mu.Lock()
	player.audioBuffer.externalPlayhead = 0
	player.audioBuffer.internalPlayhead = 2
	player.audioBuffer.lastMarkTimestamp = time.Now().Add(-2 * time.Second)
	player.audioBuffer.mu.Unlock()

	emitPlaybackProgress(player)

	if len(frames) != 1 {
		t.Fatalf("expected one playback frame event, got %d", len(frames))
	}
	if !bytes.Equal(frames[0], []byte{1, 2, 3, 4}) {
		t.Fatalf("expected playback frame %v, got %v", []byte{1, 2, 3, 4}, frames[0])
	}
}

func TestSpeechPlayerEmitApproximatePlaybackFrameSkipsRegression(t *testing.T) {
	player := newSpeechPlayer()
	player.InitBuffers(audio.GetDefaultEncodingInfo(), "")

	frames := [][]byte{}
	player.SetEventEmitter(func(event events.Event) {
		if playbackFrame, ok := event.(events.AssistantPlaybackFrame); ok {
			frames = append(frames, append([]byte(nil), playbackFrame.Audio...))
		}
	})

	player.AddAudio([]byte{1, 2})
	player.AddAudio([]byte{3, 4})

	player.audioBuffer.mu.Lock()
	player.audioBuffer.externalPlayhead = 0
	player.audioBuffer.internalPlayhead = 2
	player.audioBuffer.lastMarkTimestamp = time.Now().Add(-2 * time.Second)
	player.audioBuffer.mu.Unlock()

	emitPlaybackProgress(player)

	player.audioBuffer.mu.Lock()
	player.audioBuffer.paused = true
	player.audioBuffer.externalPlayhead = 0
	player.audioBuffer.internalPlayhead = 1
	player.audioBuffer.mu.Unlock()

	emitPlaybackProgress(player)

	if len(frames) != 1 {
		t.Fatalf("expected regression to not emit extra playback frame, got %d", len(frames))
	}
}
