package elevenlabs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/koscakluka/ema-core/core/texttospeech"
)

func TestSpeechGeneratorStreamsAudioAndMarks(t *testing.T) {
	server := newElevenLabsTestServer(t, true)
	defer server.Close()

	client, err := NewTextToSpeechClient(context.Background(), "voice-test",
		WithAPIKey("test-api-key"),
		WithBaseURL(server.websocketBaseURL()),
		WithModelID("eleven_flash_v2_5"),
	)
	if err != nil {
		t.Fatalf("expected client creation to pass, got error: %v", err)
	}

	var mu sync.Mutex
	markCallbacks := []string{}
	audioCallbacks := 0
	ended := make(chan struct{}, 1)

	generator, err := client.NewSpeechGeneratorV0(context.Background(),
		texttospeech.WithSpeechAudioCallback(func(audio []byte) {
			if len(audio) == 0 {
				return
			}
			mu.Lock()
			audioCallbacks++
			mu.Unlock()
		}),
		texttospeech.WithSpeechMarkCallback(func(transcript string) {
			mu.Lock()
			markCallbacks = append(markCallbacks, transcript)
			mu.Unlock()
		}),
		texttospeech.WithSpeechEndedCallbackV0(func(texttospeech.SpeechEndedReport) {
			select {
			case ended <- struct{}{}:
			default:
			}
		}),
	)
	if err != nil {
		t.Fatalf("expected speech generator creation to pass, got error: %v", err)
	}

	if err := generator.SendText("hello "); err != nil {
		t.Fatalf("expected SendText to pass, got error: %v", err)
	}
	if err := generator.SendText("world"); err != nil {
		t.Fatalf("expected SendText to pass, got error: %v", err)
	}
	if err := generator.Mark(); err != nil {
		t.Fatalf("expected Mark to pass, got error: %v", err)
	}
	if err := generator.EndOfText(); err != nil {
		t.Fatalf("expected EndOfText to pass, got error: %v", err)
	}

	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for speech end callback")
	}

	mu.Lock()
	defer mu.Unlock()

	if audioCallbacks == 0 {
		t.Fatalf("expected at least one audio callback")
	}
	if len(markCallbacks) != 1 {
		t.Fatalf("expected one mark callback, got %d", len(markCallbacks))
	}
	if markCallbacks[0] != "hello world" {
		t.Fatalf("expected mark transcript %q, got %q", "hello world", markCallbacks[0])
	}
}

func TestSpeechGeneratorEmitsPendingMarksOnFinalWithoutAlignment(t *testing.T) {
	server := newElevenLabsTestServer(t, false)
	defer server.Close()

	client, err := NewTextToSpeechClient(context.Background(), "voice-test",
		WithAPIKey("test-api-key"),
		WithBaseURL(server.websocketBaseURL()),
	)
	if err != nil {
		t.Fatalf("expected client creation to pass, got error: %v", err)
	}

	var mu sync.Mutex
	markCallbacks := []string{}
	ended := make(chan struct{}, 1)

	generator, err := client.NewSpeechGeneratorV0(context.Background(),
		texttospeech.WithSpeechMarkCallback(func(transcript string) {
			mu.Lock()
			markCallbacks = append(markCallbacks, transcript)
			mu.Unlock()
		}),
		texttospeech.WithSpeechEndedCallbackV0(func(texttospeech.SpeechEndedReport) {
			select {
			case ended <- struct{}{}:
			default:
			}
		}),
	)
	if err != nil {
		t.Fatalf("expected speech generator creation to pass, got error: %v", err)
	}

	if err := generator.SendText("final-only mark"); err != nil {
		t.Fatalf("expected SendText to pass, got error: %v", err)
	}
	if err := generator.Mark(); err != nil {
		t.Fatalf("expected Mark to pass, got error: %v", err)
	}
	if err := generator.EndOfText(); err != nil {
		t.Fatalf("expected EndOfText to pass, got error: %v", err)
	}

	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for speech end callback")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(markCallbacks) != 1 {
		t.Fatalf("expected one mark callback, got %d", len(markCallbacks))
	}
	if markCallbacks[0] != "final-only mark" {
		t.Fatalf("expected mark transcript %q, got %q", "final-only mark", markCallbacks[0])
	}
}

type elevenLabsTestServer struct {
	t                *testing.T
	server           *httptest.Server
	includeAlignment bool
}

func newElevenLabsTestServer(t *testing.T, includeAlignment bool) *elevenLabsTestServer {
	t.Helper()

	stub := &elevenLabsTestServer{t: t, includeAlignment: includeAlignment}
	upgrader := websocket.Upgrader{}

	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/stream-input") {
			http.NotFound(w, r)
			return
		}

		if got := r.Header.Get("xi-api-key"); got != "test-api-key" {
			t.Fatalf("expected xi-api-key header to be set")
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("failed to upgrade websocket: %v", err)
		}
		defer conn.Close()

		pendingText := ""
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}

			var payload map[string]any
			if err := json.Unmarshal(msg, &payload); err != nil {
				t.Fatalf("failed to decode client message: %v", err)
			}

			text, _ := payload["text"].(string)
			flush, _ := payload["flush"].(bool)

			switch {
			case text == " " && !flush:
				continue
			case flush:
				if pendingText == "" {
					continue
				}
				if err := conn.WriteJSON(stub.audioMessageFor(pendingText)); err != nil {
					t.Fatalf("failed to write audio message: %v", err)
				}
				pendingText = ""
			case text == "":
				if pendingText != "" {
					if err := conn.WriteJSON(stub.audioMessageFor(pendingText)); err != nil {
						t.Fatalf("failed to write final audio message: %v", err)
					}
					pendingText = ""
				}
				if err := conn.WriteJSON(map[string]any{"isFinal": true}); err != nil {
					t.Fatalf("failed to write final message: %v", err)
				}
				return
			default:
				pendingText += text
			}
		}
	}))

	return stub
}

func (s *elevenLabsTestServer) Close() {
	if s != nil && s.server != nil {
		s.server.Close()
	}
}

func (s *elevenLabsTestServer) websocketBaseURL() string {
	return strings.Replace(s.server.URL, "http://", "ws://", 1)
}

func (s *elevenLabsTestServer) audioMessageFor(text string) map[string]any {
	message := map[string]any{
		"audio": base64.StdEncoding.EncodeToString([]byte{1, 2, 3}),
	}

	if s.includeAlignment {
		message["alignment"] = alignmentPayloadForTest(text)
	}

	return message
}

func alignmentPayloadForTest(text string) map[string]any {
	chars := splitChars(text)
	starts := make([]int, 0, len(chars))
	durations := make([]int, 0, len(chars))
	for i := range chars {
		starts = append(starts, i)
		durations = append(durations, 1)
	}

	return map[string]any{
		"chars":            chars,
		"charStartTimesMs": starts,
		"charDurationsMs":  durations,
	}
}
