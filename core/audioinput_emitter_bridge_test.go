package orchestration

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/koscakluka/ema-core/core/speechtotext"
)

func TestAudioInputEmitterBridgeForwardsToCallbackAndSTT(t *testing.T) {
	expected := [][]byte{{0x01, 0x02}, {0x03, 0x04}}
	sttClient := &recordingSpeechToTextClient{}

	o := NewOrchestrator(
		WithAudioInput(&scriptedAudioInputClient{chunks: expected}),
		WithSpeechToTextClient(sttClient),
	)
	defer o.Close()

	received := make(chan []byte, len(expected))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o.Orchestrate(ctx, WithInputAudioCallback(func(audio []byte) {
		copied := append([]byte(nil), audio...)
		select {
		case received <- copied:
		default:
		}
	}))

	for i, want := range expected {
		select {
		case got := <-received:
			if !bytes.Equal(got, want) {
				t.Fatalf("expected callback audio chunk %d to be %v, got %v", i, want, got)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for callback audio chunk %d", i)
		}
	}

	deadline := time.Now().Add(2 * time.Second)
	for len(sttClient.snapshot()) < len(expected) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	sent := sttClient.snapshot()
	if len(sent) != len(expected) {
		t.Fatalf("expected %d stt audio chunks, got %d", len(expected), len(sent))
	}
	for i, want := range expected {
		if !bytes.Equal(sent[i], want) {
			t.Fatalf("expected stt audio chunk %d to be %v, got %v", i, want, sent[i])
		}
	}
}

type recordingSpeechToTextClient struct {
	mu   sync.Mutex
	sent [][]byte
}

func (client *recordingSpeechToTextClient) Transcribe(context.Context, ...speechtotext.TranscriptionOption) error {
	return nil
}

func (client *recordingSpeechToTextClient) SendAudio(audio []byte) error {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.sent = append(client.sent, append([]byte(nil), audio...))
	return nil
}

func (client *recordingSpeechToTextClient) snapshot() [][]byte {
	client.mu.Lock()
	defer client.mu.Unlock()

	cloned := make([][]byte, len(client.sent))
	for i, chunk := range client.sent {
		cloned[i] = append([]byte(nil), chunk...)
	}

	return cloned
}
