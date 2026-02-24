package orchestration

import (
	"testing"

	"github.com/koscakluka/ema-core/core/audio"
	events "github.com/koscakluka/ema-core/core/events"
)

func setTextSegments(player *speechPlayer, segments ...string) {
	player.text = append([]string(nil), segments...)
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

	player.ConfirmMark()
	if got := player.SpokenTextSoFar(); got != "Hello" {
		t.Fatalf("expected spoken text %q after first mark, got %q", "Hello", got)
	}

	player.ConfirmMark()
	if got := player.SpokenTextSoFar(); got != "Hello world" {
		t.Fatalf("expected spoken text %q after second mark, got %q", "Hello world", got)
	}
}

func TestSpeechPlayerConfirmMarkDoesNotOverrun(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello", "")
	player.ConfirmMark()
	player.ConfirmMark()
	player.ConfirmMark()

	if got := player.SpokenTextSoFar(); got != "Hello" {
		t.Fatalf("expected spoken text to remain %q when over-confirmed, got %q", "Hello", got)
	}
}

func TestSpeechPlayerApproximateSpokenTextSoFarIncludesCurrentSegment(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello", " world")

	if got := player.ApproximateSpokenTextSoFar(0.5); got != "He" {
		t.Fatalf("expected approximate spoken text %q, got %q", "He", got)
	}

	player.ConfirmMark()
	if got := player.ApproximateSpokenTextSoFar(0.5); got != "Hello wo" {
		t.Fatalf("expected approximate spoken text %q, got %q", "Hello wo", got)
	}
}

func TestSpeechPlayerApproximateSpokenTextSoFarClampsProgress(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello", " world")
	player.ConfirmMark()

	if got := player.ApproximateSpokenTextSoFar(-1); got != "Hello" {
		t.Fatalf("expected clamped lower bound result %q, got %q", "Hello", got)
	}
	if got := player.ApproximateSpokenTextSoFar(2); got != "Hello world" {
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

	player.EmitApproximateSpokenText(0.5)
	player.ConfirmMark()
	player.EmitApproximateSpokenText(0.5)

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

	player.EmitApproximateSpokenText(0.5)
	player.EmitApproximateSpokenText(0.5)
	player.EmitApproximateSpokenText(0.5)

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

	player.EmitApproximateSpokenText(0.2)
	player.EmitApproximateSpokenText(0.6)

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

	player.EmitApproximateSpokenText(1)
	player.EmitApproximateSpokenText(0.2)

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

	player.OnAudioEnded("full generated transcript")

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

	snapshot.OnAudioEnded("new turn transcript")
	if len(audioEnded) != 1 || audioEnded[0] != "new turn transcript" {
		t.Fatalf("expected snapshot audio-ended transcript %q, got %v", "new turn transcript", audioEnded)
	}

	setTextSegments(snapshot, "Hello")
	snapshot.EmitApproximateSpokenText(1)
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

	player.AddAudioChunk([]byte{1, 2, 3})
	player.AddAudioMark("Hello")

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

	transcript := player.OnAudioOutputMarkPlayed(markID)
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
	player.SetEventEmitter(func(event events.Event) {
		if spokenText, ok := event.(events.AssistantPlaybackTranscriptUpdated); ok {
			updates = append(updates, spokenText.Transcript)
		}
	})

	player.AddAudioChunk([]byte{1, 2, 3})
	player.AddAudioMark("Hello")

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

	transcript := player.OnAudioOutputMarkPlayed(markID)
	if transcript == nil || *transcript != "Hello" {
		t.Fatalf("expected combined mark handling transcript %q, got %v", "Hello", transcript)
	}

	if got := player.SpokenTextSoFar(); got != "Hello" {
		t.Fatalf("expected spoken text to advance to %q, got %q", "Hello", got)
	}
	if len(updates) != 1 || updates[0] != "Hello" {
		t.Fatalf("expected one spoken-text emission %q, got %v", "Hello", updates)
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

	player.AddAudioChunk([]byte{1, 2, 3})
	player.AddAudioMark("Hello")

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

	if transcript := player.OnAudioOutputMarkPlayed("unknown-mark"); transcript != nil {
		t.Fatalf("expected unknown mark to return nil transcript, got %q", *transcript)
	}
	if got := player.SpokenTextSoFar(); got != "" {
		t.Fatalf("expected unknown mark to not advance spoken text, got %q", got)
	}

	first := player.OnAudioOutputMarkPlayed(markID)
	if first == nil || *first != "Hello" {
		t.Fatalf("expected first confirmation transcript %q, got %v", "Hello", first)
	}

	second := player.OnAudioOutputMarkPlayed(markID)
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
