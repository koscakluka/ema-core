package elevenlabs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewTextToSpeechClientUsesTopDefaultVoice(t *testing.T) {
	t.Setenv(envVarDefaultVoiceIDName, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("search"); got != "" {
			t.Fatalf("expected empty search query, got %q", got)
		}
		if got := r.URL.Query().Get("page_size"); got != "1" {
			t.Fatalf("expected page_size query %q, got %q", "1", got)
		}
		if got := r.URL.Query().Get("voice_type"); got != string(VoiceTypeDefault) {
			t.Fatalf("expected voice_type query %q, got %q", VoiceTypeDefault, got)
		}

		response := SearchVoicesResponse{Voices: []Voice{{VoiceID: "top-default-voice"}, {VoiceID: "second-voice"}}}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewTextToSpeechClient(context.Background(), "",
		WithAPIKey("test-api-key"),
		WithBaseURL(server.URL),
	)
	if err != nil {
		t.Fatalf("expected client creation to pass, got error: %v", err)
	}

	if got := client.VoiceID(); got != "top-default-voice" {
		t.Fatalf("expected searched voice id %q, got %q", "top-default-voice", got)
	}
}

func TestNewTextToSpeechClientUsesDefaultVoiceIDFromEnv(t *testing.T) {
	t.Setenv(envVarDefaultVoiceIDName, "env-voice")

	client, err := NewTextToSpeechClient(context.Background(), "")
	if err != nil {
		t.Fatalf("expected client creation to pass, got error: %v", err)
	}

	if got := client.VoiceID(); got != "env-voice" {
		t.Fatalf("expected env voice id %q, got %q", "env-voice", got)
	}
}

func TestNewTextToSpeechClientIgnoresConfiguredVoiceTypeForDefaultResolution(t *testing.T) {
	t.Setenv(envVarDefaultVoiceIDName, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("voice_type"); got != string(VoiceTypeDefault) {
			t.Fatalf("expected voice_type query %q, got %q", VoiceTypeDefault, got)
		}

		response := SearchVoicesResponse{Voices: []Voice{{VoiceID: "default-voice"}}}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, err := NewTextToSpeechClient(context.Background(), "",
		WithAPIKey("test-api-key"),
		WithBaseURL(server.URL),
		WithVoiceType(VoiceTypeCommunity),
	)
	if err != nil {
		t.Fatalf("expected client creation to pass, got error: %v", err)
	}

	if got := client.VoiceID(); got != "default-voice" {
		t.Fatalf("expected searched voice id %q, got %q", "default-voice", got)
	}
}

func TestSetVoiceID(t *testing.T) {
	t.Setenv(envVarDefaultVoiceIDName, "")

	client, err := NewTextToSpeechClient(context.Background(), "voice-1")
	if err != nil {
		t.Fatalf("expected client creation to pass, got error: %v", err)
	}

	if err := client.SetVoiceID("voice-2"); err != nil {
		t.Fatalf("expected SetVoiceID to pass, got error: %v", err)
	}
	if got := client.VoiceID(); got != "voice-2" {
		t.Fatalf("expected voice id %q, got %q", "voice-2", got)
	}

	if err := client.SetVoiceID("   "); err == nil {
		t.Fatalf("expected SetVoiceID to fail for empty id")
	}
}

func TestNewTextToSpeechClientFailsWhenDefaultSearchReturnsNoVoices(t *testing.T) {
	t.Setenv(envVarDefaultVoiceIDName, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := SearchVoicesResponse{Voices: []Voice{}}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	_, err := NewTextToSpeechClient(context.Background(), "",
		WithAPIKey("test-api-key"),
		WithBaseURL(server.URL),
	)
	if err == nil {
		t.Fatalf("expected client creation to fail when default voice search has no results")
	}
}
