package orchestration

import (
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/koscakluka/ema-core/core/audio"
	events "github.com/koscakluka/ema-core/core/events"
)

func TestSpeechPlayerTextOrMarksWithoutSegmentationMarks(t *testing.T) {
	player := newSpeechPlayerForTest("")

	items := addAndDrainText(player, "Hello", " world")

	if len(items) != 2 {
		t.Fatalf("expected 2 text items, got %d", len(items))
	}
	if items[0].Type != "text" || items[0].Text != "Hello" {
		t.Fatalf("expected first text item %q, got %#v", "Hello", items[0])
	}
	if items[1].Type != "text" || items[1].Text != " world" {
		t.Fatalf("expected second text item %q, got %#v", " world", items[1])
	}
	if got := player.FullText(); got != "Hello world" {
		t.Fatalf("expected full text %q, got %q", "Hello world", got)
	}
}

func TestSpeechPlayerTextOrMarksEmitsBoundaryAndTrailingMark(t *testing.T) {
	player := newSpeechPlayerForTest("?.!")

	items := addAndDrainText(player, "Hello.")

	if len(items) != 3 {
		t.Fatalf("expected text + boundary mark + trailing mark, got %d items", len(items))
	}
	if items[0].Type != "text" || items[0].Text != "Hello." {
		t.Fatalf("expected first item to be text %q, got %#v", "Hello.", items[0])
	}
	if items[1].Type != "mark" {
		t.Fatalf("expected second item to be mark, got %#v", items[1])
	}
	if items[2].Type != "mark" {
		t.Fatalf("expected third item to be mark, got %#v", items[2])
	}
}

func TestSpeechPlayerTextOrMarksEmitsOnlyTrailingMarkWhenNoBoundarySeen(t *testing.T) {
	player := newSpeechPlayerForTest("?.!")

	items := addAndDrainText(player, "Hello world")

	if len(items) != 2 {
		t.Fatalf("expected text + trailing mark, got %d items", len(items))
	}
	if items[0].Type != "text" || items[0].Text != "Hello world" {
		t.Fatalf("expected first item to be text %q, got %#v", "Hello world", items[0])
	}
	if items[1].Type != "mark" {
		t.Fatalf("expected second item to be trailing mark, got %#v", items[1])
	}
}

func TestSpeechPlayerConfirmOutputMarkReturnsTranscriptAndAdvancesSpokenText(t *testing.T) {
	player := newSpeechPlayerForTest("")
	recorder := &speechPlayerEventRecorder{}
	player.SetEventEmitter(recorder.emit)

	addAndDrainText(player, "Hello")
	player.AddAudio([]byte{1, 2, 3})
	player.AddMark()

	markIDs := collectMarkIDs(player, 1)
	if len(markIDs) != 1 {
		t.Fatalf("expected one emitted mark id, got %d", len(markIDs))
	}

	transcript := player.ConfirmOutputMark(markIDs[0])
	if transcript == nil || *transcript != "Hello" {
		t.Fatalf("expected confirmed transcript %q, got %v", "Hello", transcript)
	}
	if got := player.SpokenTextSoFar(); got != "Hello" {
		t.Fatalf("expected spoken text %q after confirmation, got %q", "Hello", got)
	}

	markEvents := recorder.markPlayedEvents()
	if len(markEvents) != 1 {
		t.Fatalf("expected one mark played event, got %d", len(markEvents))
	}
	if markEvents[0].Mark != markIDs[0] {
		t.Fatalf("expected mark played id %q, got %q", markIDs[0], markEvents[0].Mark)
	}
	if markEvents[0].Transcript != "Hello" {
		t.Fatalf("expected mark played transcript %q, got %q", "Hello", markEvents[0].Transcript)
	}
}

func TestSpeechPlayerConfirmOutputMarkIgnoresUnknownAndDuplicateMarkIDs(t *testing.T) {
	player := newSpeechPlayerForTest("")

	addAndDrainText(player, "Hello")
	player.AddAudio([]byte{1, 2, 3})
	player.AddMark()

	markIDs := collectMarkIDs(player, 1)
	if len(markIDs) != 1 {
		t.Fatalf("expected one emitted mark id, got %d", len(markIDs))
	}

	if transcript := player.ConfirmOutputMark("unknown-mark"); transcript != nil {
		t.Fatalf("expected unknown mark to return nil transcript, got %q", *transcript)
	}
	if got := player.SpokenTextSoFar(); got != "" {
		t.Fatalf("expected unknown mark to not advance spoken text, got %q", got)
	}

	first := player.ConfirmOutputMark(markIDs[0])
	if first == nil || *first != "Hello" {
		t.Fatalf("expected first confirmation transcript %q, got %v", "Hello", first)
	}

	second := player.ConfirmOutputMark(markIDs[0])
	if second != nil {
		t.Fatalf("expected duplicate mark to return nil transcript, got %q", *second)
	}

	if got := player.SpokenTextSoFar(); got != "Hello" {
		t.Fatalf("expected spoken text to remain %q after duplicate mark, got %q", "Hello", got)
	}
}

func TestSpeechPlayerSpokenTextSoFarFollowsConfirmedMarkOrder(t *testing.T) {
	player := newSpeechPlayerForTest("?.!")

	addAndDrainText(player, "Hello.", " world.")

	player.AddAudio([]byte{1})
	player.AddMark()
	player.AddAudio([]byte{2})
	player.AddMark()

	markIDs := collectMarkIDs(player, 2)
	if len(markIDs) != 2 {
		t.Fatalf("expected two emitted mark ids, got %d", len(markIDs))
	}

	if got := player.SpokenTextSoFar(); got != "" {
		t.Fatalf("expected no spoken text before mark confirmations, got %q", got)
	}

	first := player.ConfirmOutputMark(markIDs[0])
	if first == nil || *first != "Hello." {
		t.Fatalf("expected first transcript %q, got %v", "Hello.", first)
	}
	if got := player.SpokenTextSoFar(); got != "Hello." {
		t.Fatalf("expected spoken text %q after first mark, got %q", "Hello.", got)
	}

	second := player.ConfirmOutputMark(markIDs[1])
	if second == nil || *second != " world." {
		t.Fatalf("expected second transcript %q, got %v", " world.", second)
	}
	if got := player.SpokenTextSoFar(); got != "Hello. world." {
		t.Fatalf("expected spoken text %q after second mark, got %q", "Hello. world.", got)
	}
}

func TestSpeechPlayerConfirmOutputMarkEmitsBufferedDeltaWithoutFurtherProgress(t *testing.T) {
	player := newSpeechPlayerForTest("")
	recorder := &speechPlayerEventRecorder{}
	player.SetEventEmitter(recorder.emit)

	addAndDrainText(player, "Hello")
	player.AddAudio(make([]byte, 320000))
	player.AddMark()

	markIDs := collectMarkIDs(player, 1)
	if len(markIDs) != 1 {
		t.Fatalf("expected one emitted mark id, got %d", len(markIDs))
	}

	transcript := player.ConfirmOutputMark(markIDs[0])
	if transcript == nil || *transcript != "Hello" {
		t.Fatalf("expected confirmed transcript %q, got %v", "Hello", transcript)
	}

	segments := recorder.transcriptSegmentEvents()
	if len(segments) == 0 {
		t.Fatalf("expected spoken delta event to be emitted")
	}
	if got := segments[len(segments)-1]; got != "Hello" {
		t.Fatalf("expected last spoken delta %q, got %q", "Hello", got)
	}

	updates := recorder.transcriptUpdateEvents()
	if len(updates) == 0 {
		t.Fatalf("expected spoken text update event to be emitted")
	}
	if got := updates[len(updates)-1]; got != "Hello" {
		t.Fatalf("expected last spoken text update %q, got %q", "Hello", got)
	}
}

func TestGetPercentOfKeepsUTF8Boundaries(t *testing.T) {
	text := "A🙂Б"
	for i := 0; i <= 100; i++ {
		partial := getPercentOf(text, float64(i)/100)
		if !utf8.ValidString(partial) {
			t.Fatalf("expected valid UTF-8 for percent %d, got %q", i, partial)
		}
	}

	if got := getPercentOf(text, 0.67); got != "A🙂" {
		t.Fatalf("expected rune-safe partial %q, got %q", "A🙂", got)
	}
}

func TestSpeechPlayerAudioEmitsPlaybackStartedWhenAudioIsConsumed(t *testing.T) {
	player := newSpeechPlayerForTest("")
	recorder := &speechPlayerEventRecorder{}
	player.SetEventEmitter(recorder.emit)

	player.AddAudio([]byte{1, 2, 3})
	player.AddMark()

	player.Audio(func(item audioOrMark) bool {
		return item.Type == "audio"
	})

	if started := recorder.startedCount(); started != 1 {
		t.Fatalf("expected one playback started event, got %d", started)
	}
}

func TestSpeechPlayerAudioSkipsPlaybackStartedWhenFirstItemRejected(t *testing.T) {
	player := newSpeechPlayerForTest("")
	recorder := &speechPlayerEventRecorder{}
	player.SetEventEmitter(recorder.emit)

	player.AddAudio([]byte{1, 2, 3})
	player.AddMark()

	player.Audio(func(item audioOrMark) bool {
		_ = item
		return false
	})

	if started := recorder.startedCount(); started != 0 {
		t.Fatalf("expected no playback started event, got %d", started)
	}
}

func TestSpeechPlayerAudioEmitsPlaybackEndedWithFullText(t *testing.T) {
	player := newSpeechPlayerForTest("")
	recorder := &speechPlayerEventRecorder{}
	player.SetEventEmitter(recorder.emit)

	player.AddTextChunk("full generated transcript")
	player.TextComplete()

	player.AddAudio([]byte{1, 2, 3})
	player.AddMark()

	player.Audio(func(item audioOrMark) bool {
		_ = item
		return false
	})

	ended := recorder.endedTranscripts()
	if len(ended) != 1 || ended[0] != "full generated transcript" {
		t.Fatalf("expected one playback ended transcript %q, got %v", "full generated transcript", ended)
	}
}

func TestSpeechPlayerSnapshotKeepsEmitterAndStartsWithFreshState(t *testing.T) {
	player := newSpeechPlayerForTest("")
	recorder := &speechPlayerEventRecorder{}
	player.SetEventEmitter(recorder.emit)

	addAndDrainText(player, "already queued")
	player.AddAudio([]byte{1})
	player.AddMark()

	markIDs := collectMarkIDs(player, 1)
	if len(markIDs) != 1 {
		t.Fatalf("expected one emitted mark id, got %d", len(markIDs))
	}
	if transcript := player.ConfirmOutputMark(markIDs[0]); transcript == nil || *transcript != "already queued" {
		t.Fatalf("expected initial transcript %q, got %v", "already queued", transcript)
	}

	snapshot := player.Snapshot()
	if snapshot == nil {
		t.Fatalf("expected snapshot to be created")
	}
	if got := snapshot.SpokenTextSoFar(); got != "" {
		t.Fatalf("expected snapshot spoken text to start empty, got %q", got)
	}

	before := len(recorder.markPlayedEvents())

	snapshot.InitBuffers(audio.GetDefaultEncodingInfo(), "")
	addAndDrainText(snapshot, "new turn transcript")
	snapshot.AddAudio([]byte{9})
	snapshot.AddMark()

	snapshotMarkIDs := collectMarkIDs(snapshot, 1)
	if len(snapshotMarkIDs) != 1 {
		t.Fatalf("expected snapshot to emit one mark id, got %d", len(snapshotMarkIDs))
	}

	snapshotTranscript := snapshot.ConfirmOutputMark(snapshotMarkIDs[0])
	if snapshotTranscript == nil || *snapshotTranscript != "new turn transcript" {
		t.Fatalf("expected snapshot transcript %q, got %v", "new turn transcript", snapshotTranscript)
	}

	afterEvents := recorder.markPlayedEvents()
	if len(afterEvents) != before+1 {
		t.Fatalf("expected snapshot confirmation to emit one additional mark event, got %d -> %d", before, len(afterEvents))
	}
	last := afterEvents[len(afterEvents)-1]
	if last.Transcript != "new turn transcript" {
		t.Fatalf("expected last mark event transcript %q, got %q", "new turn transcript", last.Transcript)
	}
}

func newSpeechPlayerForTest(segmentationBoundaries string) *speechPlayer {
	player := newSpeechPlayer()
	player.InitBuffers(audio.GetDefaultEncodingInfo(), segmentationBoundaries)
	return player
}

func addAndDrainText(player *speechPlayer, chunks ...string) []textOrMark {
	for _, chunk := range chunks {
		player.AddTextChunk(chunk)
	}
	player.TextComplete()

	items := []textOrMark{}
	for item := range player.TextOrMarks {
		items = append(items, item)
	}

	return items
}

func collectMarkIDs(player *speechPlayer, count int) []string {
	markIDs := []string{}
	player.Audio(func(item audioOrMark) bool {
		if item.Type == "mark" {
			markIDs = append(markIDs, item.Mark)
			if len(markIDs) >= count {
				return false
			}
		}
		return true
	})

	return markIDs
}

type speechPlayerEventRecorder struct {
	mu                 sync.Mutex
	started            int
	ended              []string
	markPlayed         []events.AssistantPlaybackMarkPlayed
	transcriptUpdates  []string
	transcriptSegments []string
}

func (recorder *speechPlayerEventRecorder) emit(event events.Event) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()

	switch typedEvent := event.(type) {
	case events.AssistantPlaybackStarted:
		recorder.started++
	case events.AssistantPlaybackEnded:
		recorder.ended = append(recorder.ended, typedEvent.Transcript)
	case events.AssistantPlaybackMarkPlayed:
		recorder.markPlayed = append(recorder.markPlayed, typedEvent)
	case events.AssistantPlaybackTranscriptUpdated:
		recorder.transcriptUpdates = append(recorder.transcriptUpdates, typedEvent.Transcript)
	case events.AssistantPlaybackTranscriptSegment:
		recorder.transcriptSegments = append(recorder.transcriptSegments, typedEvent.Segment)
	}
}

func (recorder *speechPlayerEventRecorder) startedCount() int {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return recorder.started
}

func (recorder *speechPlayerEventRecorder) endedTranscripts() []string {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	transcripts := make([]string, len(recorder.ended))
	copy(transcripts, recorder.ended)
	return transcripts
}

func (recorder *speechPlayerEventRecorder) markPlayedEvents() []events.AssistantPlaybackMarkPlayed {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	eventsCopy := make([]events.AssistantPlaybackMarkPlayed, len(recorder.markPlayed))
	copy(eventsCopy, recorder.markPlayed)
	return eventsCopy
}

func (recorder *speechPlayerEventRecorder) transcriptUpdateEvents() []string {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	updates := make([]string, len(recorder.transcriptUpdates))
	copy(updates, recorder.transcriptUpdates)
	return updates
}

func (recorder *speechPlayerEventRecorder) transcriptSegmentEvents() []string {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	segments := make([]string, len(recorder.transcriptSegments))
	copy(segments, recorder.transcriptSegments)
	return segments
}
