package orchestration

import "testing"

func TestSpeechPlayerAddTextTracksCurrentSegment(t *testing.T) {
	player := newSpeechPlayer()

	player.AddText("Hello")
	player.AddText(" world")

	if len(player.text) != 1 {
		t.Fatalf("expected one text segment, got %d", len(player.text))
	}
	if got := player.text[0]; got != "Hello world" {
		t.Fatalf("expected segment text %q, got %q", "Hello world", got)
	}
}

func TestSpeechPlayerMarkStartsNewSegment(t *testing.T) {
	player := newSpeechPlayer()

	player.AddText("Hello")
	player.Mark()
	player.AddText(" world")

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

	player.AddText("Hello")
	player.Mark()
	player.AddText(" world")
	player.Mark()
	player.AddText("!")

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

	player.AddText("Hello")
	player.Mark()
	player.ConfirmMark()
	player.ConfirmMark()
	player.ConfirmMark()

	if got := player.SpokenTextSoFar(); got != "Hello" {
		t.Fatalf("expected spoken text to remain %q when over-confirmed, got %q", "Hello", got)
	}
}

func TestSpeechPlayerApproximateSpokenTextSoFarIncludesCurrentSegment(t *testing.T) {
	player := newSpeechPlayer()

	player.AddText("Hello")
	player.Mark()
	player.AddText(" world")

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

	player.AddText("Hello")
	player.Mark()
	player.AddText(" world")
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

	player.AddText("Hello")
	player.Mark()
	player.AddText(" world")

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

	player.AddText("Hello")

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

	player.AddText("Hello")

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

	player.AddText("Hello")

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
	player.AddText("already queued")

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

	snapshot.AddText("Hello")
	snapshot.EmitApproximateSpokenText(1)
	if spokenCalled != "Hello" {
		t.Fatalf("expected snapshot spoken-text callback %q, got %q", "Hello", spokenCalled)
	}
	if spokenDeltaCalled != "Hello" {
		t.Fatalf("expected snapshot spoken-text delta callback %q, got %q", "Hello", spokenDeltaCalled)
	}
}
