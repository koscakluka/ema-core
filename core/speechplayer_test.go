package orchestration

import (
	"testing"

	"github.com/koscakluka/ema-core/core/audio"
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

func TestSpeechPlayerEmitApproximateSpokenTextCallsCallback(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello", " world")

	updates := []string{}
	player.SetSpokenTextCallback(func(spokenText string) {
		updates = append(updates, spokenText)
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
	player.SetSpokenTextCallback(func(spokenText string) {
		updates = append(updates, spokenText)
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
	player.SetSpokenTextDeltaCallback(func(spokenTextDelta string) {
		deltas = append(deltas, spokenTextDelta)
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

func TestSpeechPlayerEmitApproximateSpokenTextDeltaFallsBackToReplacement(t *testing.T) {
	player := newSpeechPlayer()

	setTextSegments(player, "Hello")

	deltas := []string{}
	player.SetSpokenTextDeltaCallback(func(spokenTextDelta string) {
		deltas = append(deltas, spokenTextDelta)
	})

	player.EmitApproximateSpokenText(1)
	player.EmitApproximateSpokenText(0.2)

	if len(deltas) != 2 {
		t.Fatalf("expected 2 spoken text deltas, got %d", len(deltas))
	}
	if deltas[0] != "Hello" {
		t.Fatalf("expected first delta %q, got %q", "Hello", deltas[0])
	}
	if deltas[1] != "H" {
		t.Fatalf("expected replacement delta %q, got %q", "H", deltas[1])
	}
}

func TestSpeechPlayerOnAudioEndedFallsBackToProvidedTranscript(t *testing.T) {
	player := newSpeechPlayer()

	called := ""
	player.SetCallbacks(func(transcript string) {
		called = transcript
	})

	player.OnAudioEnded("full generated transcript")

	if called != "full generated transcript" {
		t.Fatalf("expected fallback transcript %q, got %q", "full generated transcript", called)
	}
}

func TestSpeechPlayerSnapshotKeepsCallbacksButNotMarkedText(t *testing.T) {
	player := newSpeechPlayer()
	setTextSegments(player, "already queued")

	called := ""
	player.SetCallbacks(func(transcript string) {
		called = transcript
	})

	spokenCalled := ""
	player.SetSpokenTextCallback(func(spokenText string) {
		spokenCalled = spokenText
	})

	spokenDeltaCalled := ""
	player.SetSpokenTextDeltaCallback(func(spokenTextDelta string) {
		spokenDeltaCalled = spokenTextDelta
	})

	snapshot := player.Snapshot()
	if len(snapshot.text) != 0 {
		t.Fatalf("expected snapshot text queue to be empty, got %d segments", len(snapshot.text))
	}

	snapshot.OnAudioEnded("new turn transcript")
	if called != "new turn transcript" {
		t.Fatalf("expected snapshot callback transcript %q, got %q", "new turn transcript", called)
	}

	setTextSegments(snapshot, "Hello")
	snapshot.EmitApproximateSpokenText(1)
	if spokenCalled != "Hello" {
		t.Fatalf("expected snapshot spoken-text callback %q, got %q", "Hello", spokenCalled)
	}
	if spokenDeltaCalled != "Hello" {
		t.Fatalf("expected snapshot spoken-text delta callback %q, got %q", "Hello", spokenDeltaCalled)
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
	player.SetSpokenTextCallback(func(spokenText string) {
		updates = append(updates, spokenText)
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

func TestSpeechPlayerTextOrMarksEmitsBoundaryMarkWhenConfigured(t *testing.T) {
	player := newSpeechPlayer()
	player.InitBuffers(audio.GetDefaultEncodingInfo(), "?.!")

	player.AddTextChunk("Hello.")
	player.TextComplete()

	items := []textOrMark{}
	for item := range player.TextOrMarks {
		items = append(items, item)
	}

	if len(items) != 2 {
		t.Fatalf("expected one text and one mark event, got %d items", len(items))
	}
	if items[0].Type != textOrMarkTypeText || items[0].Text != "Hello." {
		t.Fatalf("expected first event to be text %q, got %#v", "Hello.", items[0])
	}
	if items[1].Type != textOrMarkTypeMark {
		t.Fatalf("expected second event to be mark, got %#v", items[1])
	}
	if len(player.text) != 2 {
		t.Fatalf("expected boundary segmentation to create next segment, got %d", len(player.text))
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

func TestSpeechPlayerTextOrMarksDoesNotEmitMarkWithoutBoundary(t *testing.T) {
	player := newSpeechPlayer()
	player.InitBuffers(audio.GetDefaultEncodingInfo(), "?.!")

	player.AddTextChunk("Hello world")
	player.TextComplete()

	items := []textOrMark{}
	for item := range player.TextOrMarks {
		items = append(items, item)
	}

	if len(items) != 1 {
		t.Fatalf("expected one text event without boundary, got %d", len(items))
	}
	if items[0].Type != textOrMarkTypeText || items[0].Text != "Hello world" {
		t.Fatalf("expected text event %q, got %#v", "Hello world", items[0])
	}
	if len(player.text) != 1 {
		t.Fatalf("expected no segmentation without boundary, got %d segments", len(player.text))
	}
}
