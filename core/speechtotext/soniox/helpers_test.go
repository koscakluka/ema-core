package soniox

import (
	"testing"
	"time"

	"github.com/koscakluka/ema-core/core/audio"
)

func TestNewSilenceChunkLinear16(t *testing.T) {
	chunk, err := newSilenceChunk(audio.EncodingInfo{
		SampleRate: 16000,
		Format:     audio.EncodingLinear16,
	}, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got, want := len(chunk), 1600; got != want {
		t.Fatalf("expected chunk length %d, got %d", want, got)
	}

	for i, b := range chunk {
		if b != 0 {
			t.Fatalf("expected silence byte at index %d to be 0, got %d", i, b)
		}
	}
}

func TestNewSilenceChunkALaw(t *testing.T) {
	chunk, err := newSilenceChunk(audio.EncodingInfo{
		SampleRate: 8000,
		Format:     audio.EncodingALaw,
	}, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got, want := len(chunk), 400; got != want {
		t.Fatalf("expected chunk length %d, got %d", want, got)
	}

	for i, b := range chunk {
		if b != 0x55 {
			t.Fatalf("expected silence byte at index %d to be 0x55, got %d", i, b)
		}
	}
}

func TestNewSilenceChunkRejectsInvalidEncoding(t *testing.T) {
	_, err := newSilenceChunk(audio.EncodingInfo{
		SampleRate: 16000,
	}, 50*time.Millisecond)
	if err == nil {
		t.Fatalf("expected error for invalid encoding format")
	}
}
