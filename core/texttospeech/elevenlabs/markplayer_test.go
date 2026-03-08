package elevenlabs

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"iter"
	"strings"
	"testing"

	"github.com/koscakluka/ema-core/core/audio"
)

func TestMarkerPlayLifecycleWithMultipleMarks(t *testing.T) {
	var p marker
	addMarkForTest(&p, "hello")
	addMarkForTest(&p, "world")
	p.Finalise()

	audioBytes := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	elements, err := collectMarkerOutput(p.Play(receiveMessage{
		Audio: encodeAudio(audioBytes),
		Alignment: &alignment{
			Chars:            splitChars("helloworld"),
			CharStartTimesMs: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
			CharDurationsMs:  []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		},
	}, markerTestEncoding))
	if err != nil {
		t.Fatalf("expected Play to pass, got error: %v", err)
	}

	assertMarkerElements(t, elements, []expectedMarkerElement{
		{Type: markerElementTypeAudio, Audio: audioBytes[:5]},
		{Type: markerElementTypeMark, Mark: "hello"},
		{Type: markerElementTypeAudio, Audio: audioBytes[5:]},
		{Type: markerElementTypeMark, Mark: "world"},
	})
}

func TestMarkerPlayFinaliseBeforeAnyText(t *testing.T) {
	var p marker
	p.Finalise()

	audioBytes := []byte{9, 8, 7}
	elements, err := collectMarkerOutput(p.Play(receiveMessage{
		Audio: encodeAudio(audioBytes),
		Alignment: &alignment{
			Chars:            []string{},
			CharStartTimesMs: []int{},
			CharDurationsMs:  []int{},
		},
	}, markerTestEncoding))
	if err != nil {
		t.Fatalf("expected Play to pass, got error: %v", err)
	}

	assertMarkerElements(t, elements, []expectedMarkerElement{
		{Type: markerElementTypeAudio, Audio: audioBytes},
	})
}

func TestMarkerPlayFailsForInvalidBase64Audio(t *testing.T) {
	var p marker

	elements, err := collectMarkerOutput(p.Play(receiveMessage{Audio: "invalid-base64"}, markerTestEncoding))
	if err == nil {
		t.Fatalf("expected invalid base64 audio to fail")
	}
	if !strings.Contains(err.Error(), "failed to decode audio from base64") {
		t.Fatalf("expected base64 decode error, got %v", err)
	}
	if len(elements) != 0 {
		t.Fatalf("expected no elements on decode failure, got %d", len(elements))
	}
}

func TestMarkerPlayWithNilAlignmentPassesAudioThrough(t *testing.T) {
	var p marker
	p.AddText("pending")
	p.Finalise()

	audioBytes := []byte{1, 2, 3, 4}
	elements, err := collectMarkerOutput(p.Play(receiveMessage{Audio: encodeAudio(audioBytes)}, markerTestEncoding))
	if err != nil {
		t.Fatalf("expected Play to pass with nil alignment, got error: %v", err)
	}

	assertMarkerElements(t, elements, []expectedMarkerElement{
		{Type: markerElementTypeAudio, Audio: audioBytes},
	})
}

func TestMarkerPlayCarriesOverPartialMarkAcrossMessages(t *testing.T) {
	var p marker
	addMarkForTest(&p, "hello")
	p.Finalise()

	firstAudio := []byte{1, 2}
	firstElements, err := collectMarkerOutput(p.Play(receiveMessage{
		Audio: encodeAudio(firstAudio),
		Alignment: &alignment{
			Chars:            splitChars("he"),
			CharStartTimesMs: []int{0, 1},
			CharDurationsMs:  []int{1, 1},
		},
	}, markerTestEncoding))
	if err != nil {
		t.Fatalf("expected first Play call to pass, got error: %v", err)
	}
	assertMarkerElements(t, firstElements, []expectedMarkerElement{
		{Type: markerElementTypeAudio, Audio: firstAudio},
	})

	secondAudio := []byte{3, 4, 5, 6, 7}
	secondElements, err := collectMarkerOutput(p.Play(receiveMessage{
		Audio: encodeAudio(secondAudio),
		Alignment: &alignment{
			Chars:            splitChars("llo"),
			CharStartTimesMs: []int{2, 3, 4},
			CharDurationsMs:  []int{1, 1, 1},
		},
	}, markerTestEncoding))
	if err != nil {
		t.Fatalf("expected second Play call to pass, got error: %v", err)
	}
	assertMarkerElements(t, secondElements, []expectedMarkerElement{
		{Type: markerElementTypeAudio, Audio: secondAudio[:3]},
		{Type: markerElementTypeMark, Mark: "hello"},
		{Type: markerElementTypeAudio, Audio: secondAudio[3:]},
	})
}

func TestMarkerPlayCancellationOnAudioDropsPendingMark(t *testing.T) {
	var p marker
	addMarkForTest(&p, "a")
	p.Finalise()

	seq := p.Play(receiveMessage{
		Audio: encodeAudio([]byte{10, 11, 12}),
		Alignment: &alignment{
			Chars:            splitChars("a"),
			CharStartTimesMs: []int{0},
			CharDurationsMs:  []int{1},
		},
	}, markerTestEncoding)

	callbacks := 0
	firstCallElements := make([]markerElement, 0)
	seq(func(element markerElement, err error) bool {
		if err != nil {
			t.Fatalf("expected no iterator error, got %v", err)
		}
		callbacks++
		firstCallElements = append(firstCallElements, element)
		return false
	})

	if callbacks != 1 {
		t.Fatalf("expected one callback before cancellation, got %d", callbacks)
	}
	assertMarkerElements(t, firstCallElements, []expectedMarkerElement{{Type: markerElementTypeAudio, Audio: []byte{10}}})

	followUp, err := collectMarkerOutput(p.Play(receiveMessage{
		Audio: encodeAudio([]byte{20, 21}),
		Alignment: &alignment{
			Chars:            []string{},
			CharStartTimesMs: []int{},
			CharDurationsMs:  []int{},
		},
	}, markerTestEncoding))
	if err != nil {
		t.Fatalf("expected follow-up Play call to pass, got error: %v", err)
	}
	assertMarkerElements(t, followUp, []expectedMarkerElement{{Type: markerElementTypeAudio, Audio: []byte{20, 21}}})
}

func TestMarkerPlayCancellationSkipsTwoPendingMarksAndContinuesAfterThem(t *testing.T) {
	firstMessage := receiveMessage{
		Audio: encodeAudio([]byte{1, 2, 3, 4, 5, 6}),
		Alignment: &alignment{
			Chars:            splitChars("abcd"),
			CharStartTimesMs: []int{0, 1, 2, 3},
			CharDurationsMs:  []int{1, 1, 1, 1},
		},
	}

	var baseline marker
	addMarkForTest(&baseline, "ab")
	addMarkForTest(&baseline, "cd")
	addMarkForTest(&baseline, "ef")
	baseline.Finalise()

	baselineElements, err := collectMarkerOutput(baseline.Play(firstMessage, markerTestEncoding))
	if err != nil {
		t.Fatalf("expected baseline Play call to pass, got error: %v", err)
	}
	baselineMarks := collectMarks(baselineElements)
	if len(baselineMarks) < 2 || baselineMarks[0] != "ab" || baselineMarks[1] != "cd" {
		t.Fatalf("expected baseline message to emit marks %q and %q, got %v", "ab", "cd", baselineMarks)
	}

	var p marker
	addMarkForTest(&p, "ab")
	addMarkForTest(&p, "cd")
	addMarkForTest(&p, "ef")
	p.Finalise()

	firstSeq := p.Play(firstMessage, markerTestEncoding)

	firstCallElements := make([]markerElement, 0)
	callbacks := 0
	firstSeq(func(element markerElement, err error) bool {
		if err != nil {
			t.Fatalf("expected no iterator error, got %v", err)
		}
		callbacks++
		firstCallElements = append(firstCallElements, element)
		return false
	})

	if callbacks != 1 {
		t.Fatalf("expected one callback before cancellation, got %d", callbacks)
	}
	assertMarkerElements(t, firstCallElements, []expectedMarkerElement{{Type: markerElementTypeAudio, Audio: []byte{1, 2}}})

	followUp, err := collectMarkerOutput(p.Play(receiveMessage{
		Audio: encodeAudio([]byte{7, 8}),
		Alignment: &alignment{
			Chars:            splitChars("ef"),
			CharStartTimesMs: []int{4, 5},
			CharDurationsMs:  []int{1, 1},
		},
	}, markerTestEncoding))
	if err != nil {
		t.Fatalf("expected follow-up Play call to pass, got error: %v", err)
	}

	followUpMarks := collectMarks(followUp)
	if len(followUpMarks) != 1 || followUpMarks[0] != "ef" {
		t.Fatalf("expected follow-up to emit only mark %q, got %v", "ef", followUpMarks)
	}
	if markSliceContains(followUpMarks, "ab") || markSliceContains(followUpMarks, "cd") {
		t.Fatalf("expected skipped marks %q and %q to never be emitted after interruption, got %v", "ab", "cd", followUpMarks)
	}

	followUpAudio := collectAudio(followUp)
	if !bytes.Equal(followUpAudio, []byte{7, 8}) {
		t.Fatalf("expected follow-up audio %v, got %v (%s)", []byte{7, 8}, followUpAudio, formatMarkerElements(followUp))
	}
}

func TestMarkerPlayCancellationOnMarkConsumesMark(t *testing.T) {
	var p marker
	addMarkForTest(&p, "a")
	p.Finalise()

	seq := p.Play(receiveMessage{
		Audio: encodeAudio([]byte{30, 31, 32}),
		Alignment: &alignment{
			Chars:            splitChars("a"),
			CharStartTimesMs: []int{0},
			CharDurationsMs:  []int{1},
		},
	}, markerTestEncoding)

	firstCallElements := make([]markerElement, 0)
	callbacks := 0
	seq(func(element markerElement, err error) bool {
		if err != nil {
			t.Fatalf("expected no iterator error, got %v", err)
		}
		callbacks++
		firstCallElements = append(firstCallElements, element)
		return element.Type == markerElementTypeAudio
	})

	if callbacks != 2 {
		t.Fatalf("expected two callbacks before cancellation, got %d", callbacks)
	}
	assertMarkerElements(t, firstCallElements, []expectedMarkerElement{
		{Type: markerElementTypeAudio, Audio: []byte{30}},
		{Type: markerElementTypeMark, Mark: "a"},
	})

	followUp, err := collectMarkerOutput(p.Play(receiveMessage{
		Audio: encodeAudio([]byte{40, 41}),
		Alignment: &alignment{
			Chars:            []string{},
			CharStartTimesMs: []int{},
			CharDurationsMs:  []int{},
		},
	}, markerTestEncoding))
	if err != nil {
		t.Fatalf("expected follow-up Play call to pass, got error: %v", err)
	}
	assertMarkerElements(t, followUp, []expectedMarkerElement{{Type: markerElementTypeAudio, Audio: []byte{40, 41}}})
}

var markerTestEncoding = audio.EncodingInfo{SampleRate: 1000, Format: audio.EncodingMulaw}

type expectedMarkerElement struct {
	Type  markerElementType
	Audio []byte
	Mark  string
}

func assertMarkerElements(t *testing.T, got []markerElement, want []expectedMarkerElement) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("expected %d elements, got %d (%s)", len(want), len(got), formatMarkerElements(got))
	}

	for i := range want {
		if got[i].Type != want[i].Type {
			t.Fatalf("element %d: expected type %q, got %q (%s)", i, want[i].Type, got[i].Type, formatMarkerElements(got))
		}
		if !bytes.Equal(got[i].Audio, want[i].Audio) {
			t.Fatalf("element %d: expected audio %v, got %v (%s)", i, want[i].Audio, got[i].Audio, formatMarkerElements(got))
		}
		if got[i].Mark != want[i].Mark {
			t.Fatalf("element %d: expected mark %q, got %q (%s)", i, want[i].Mark, got[i].Mark, formatMarkerElements(got))
		}
	}
}

func formatMarkerElements(elements []markerElement) string {
	parts := make([]string, 0, len(elements))
	for i, el := range elements {
		parts = append(parts, fmt.Sprintf("%d:{type:%s audio:%v mark:%q}", i, el.Type, el.Audio, el.Mark))
	}

	return strings.Join(parts, ", ")
}

func collectMarks(elements []markerElement) []string {
	marks := make([]string, 0)
	for _, el := range elements {
		if el.Type == markerElementTypeMark {
			marks = append(marks, el.Mark)
		}
	}

	return marks
}

func collectAudio(elements []markerElement) []byte {
	audio := make([]byte, 0)
	for _, el := range elements {
		if el.Type == markerElementTypeAudio && len(el.Audio) > 0 {
			audio = append(audio, el.Audio...)
		}
	}

	return audio
}

func markSliceContains(marks []string, target string) bool {
	for _, mark := range marks {
		if mark == target {
			return true
		}
	}

	return false
}

func collectMarkerOutput(seq iter.Seq2[markerElement, error]) ([]markerElement, error) {
	elements := make([]markerElement, 0)
	for element, err := range seq {
		if err != nil {
			return elements, err
		}
		elements = append(elements, element)
	}

	return elements, nil
}

func encodeAudio(audioBytes []byte) string {
	return base64.StdEncoding.EncodeToString(audioBytes)
}

func addMarkForTest(p *marker, text string) {
	p.AddText(text)
	p.NewMark()
}

func splitChars(text string) []string {
	chars := make([]string, 0, len(text))
	for _, r := range text {
		chars = append(chars, string(r))
	}

	return chars
}
