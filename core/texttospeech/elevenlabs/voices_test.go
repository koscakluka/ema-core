package elevenlabs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchVoicesUsesClientVoiceTypeByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("xi-api-key"); got != "test-api-key" {
			t.Fatalf("expected xi-api-key header to be set")
		}
		if got := r.URL.Query().Get("voice_type"); got != string(VoiceTypeCommunity) {
			t.Fatalf("expected voice_type query %q, got %q", VoiceTypeCommunity, got)
		}

		response := SearchVoicesResponse{
			Voices: []Voice{{VoiceID: "voice-1", Name: "Voice 1"}},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewTextToSpeechClient(context.Background(), "voice-default",
		WithAPIKey("test-api-key"),
		WithBaseURL(server.URL),
		WithVoiceType(VoiceTypeCommunity),
	)
	if err != nil {
		t.Fatalf("expected client creation to pass, got error: %v", err)
	}

	result, err := client.SearchVoices(context.Background(), SearchVoicesOptions{})
	if err != nil {
		t.Fatalf("expected search to pass, got error: %v", err)
	}

	if len(result.Voices) != 1 || result.Voices[0].VoiceID != "voice-1" {
		t.Fatalf("expected one voice with id %q", "voice-1")
	}
}

func TestSearchVoicesCanOverrideVoiceType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("voice_type"); got != string(VoiceTypeSaved) {
			t.Fatalf("expected voice_type query %q, got %q", VoiceTypeSaved, got)
		}

		response := SearchVoicesResponse{
			Voices: []Voice{{VoiceID: "voice-2", Name: "Voice 2"}},
		}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewTextToSpeechClient(context.Background(), "voice-default",
		WithAPIKey("test-api-key"),
		WithBaseURL(server.URL),
		WithVoiceType(VoiceTypeCommunity),
	)
	if err != nil {
		t.Fatalf("expected client creation to pass, got error: %v", err)
	}

	_, err = client.SearchVoices(context.Background(), SearchVoicesOptions{VoiceType: VoiceTypeSaved})
	if err != nil {
		t.Fatalf("expected search to pass, got error: %v", err)
	}
}

func TestSearchVoicesRejectsInvalidVoiceType(t *testing.T) {
	client, err := NewTextToSpeechClient(context.Background(), "voice-default")
	if err != nil {
		t.Fatalf("expected client creation to pass, got error: %v", err)
	}

	_, err = client.SearchVoices(context.Background(), SearchVoicesOptions{VoiceType: VoiceType("bad")})
	if err == nil {
		t.Fatalf("expected invalid voice type to fail")
	}
}
