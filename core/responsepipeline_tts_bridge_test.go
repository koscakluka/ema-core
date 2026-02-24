package orchestration

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/texttospeech"
)

func TestResponsePipelineBridgesTTSEventsToSpeechPlayerAndAudioOutput(t *testing.T) {
	output := &bridgeAudioOutputStub{}
	o := NewOrchestrator(
		WithLLM(promptLLMStub{response: "bridge coverage"}),
		WithTextToSpeechClientV1(&bridgeTTSV1Stub{}),
		WithAudioOutputV1(output),
	)
	defer o.Close()

	responseEnded := make(chan struct{}, 1)
	var callbackAudioChunks atomic.Int32
	var callbackPlaybackAudioChunks atomic.Int32

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o.Orchestrate(ctx,
		WithAudioCallback(func(audio []byte) {
			if len(audio) > 0 {
				callbackAudioChunks.Add(1)
			}
		}),
		WithPlaybackAudioCallback(func(audio []byte) {
			if len(audio) > 0 {
				callbackPlaybackAudioChunks.Add(1)
			}
		}),
		WithResponseEndCallback(func() {
			select {
			case responseEnded <- struct{}{}:
			default:
			}
		}),
	)

	o.SendPrompt("bridge prompt")

	select {
	case <-responseEnded:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for response end callback")
	}

	waitForCondition(t, 2*time.Second, "tts bridge audio output", func() bool {
		return output.nonEmptyAudioChunks() > 0
	})

	if output.nonEmptyAudioChunks() == 0 {
		t.Fatalf("expected bridged tts audio to reach audio output")
	}
	if got := callbackAudioChunks.Load(); got == 0 {
		t.Fatalf("expected WithAudioCallback to receive bridged tts audio")
	}
	waitForCondition(t, 2*time.Second, "playback audio callback", func() bool {
		return callbackPlaybackAudioChunks.Load() > 0
	})
	if got := callbackPlaybackAudioChunks.Load(); got == 0 {
		t.Fatalf("expected WithPlaybackAudioCallback to receive playback audio")
	}
}

func TestResponsePipelineUsesLegacyTTSModeWithoutLegacyEvent(t *testing.T) {
	output := &bridgeAudioOutputStub{}
	legacyTTS := &bridgeLegacyTTSStub{}
	o := NewOrchestrator(
		WithLLM(promptLLMStub{response: "legacy bridge"}),
		WithTextToSpeechClient(legacyTTS),
		WithAudioOutputV1(output),
	)
	defer o.Close()

	responseEnded := make(chan struct{}, 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	o.Orchestrate(ctx,
		WithResponseEndCallback(func() {
			select {
			case responseEnded <- struct{}{}:
			default:
			}
		}),
	)

	o.SendPrompt("legacy prompt")

	select {
	case <-responseEnded:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for response end callback")
	}

	waitForCondition(t, 2*time.Second, "legacy speech pipeline turn completion", func() bool {
		return o.currentResponsePipeline() == nil
	})

	if output.nonEmptyAudioChunks() == 0 {
		t.Fatalf("expected bridged legacy tts audio to reach audio output")
	}
	if output.marks() == 0 {
		t.Fatalf("expected bridged legacy tts marks to reach audio output")
	}
	if legacyTTS.flushCount() < 2 {
		t.Fatalf("expected legacy tts to flush at least mark and end-of-text")
	}
}

type bridgeTTSV1Stub struct{}

func (stub *bridgeTTSV1Stub) NewSpeechGeneratorV0(
	ctx context.Context,
	opts ...texttospeech.TextToSpeechOption,
) (texttospeech.SpeechGeneratorV0, error) {
	_ = stub
	_ = ctx
	config := texttospeech.TextToSpeechOptions{}
	for _, opt := range opts {
		opt(&config)
	}

	return &bridgeSpeechGeneratorStub{config: config}, nil
}

type bridgeSpeechGeneratorStub struct {
	mu      sync.Mutex
	config  texttospeech.TextToSpeechOptions
	pending strings.Builder
	closed  bool
}

type bridgeLegacyTTSStub struct {
	mu      sync.Mutex
	config  texttospeech.TextToSpeechOptions
	pending strings.Builder
	closed  bool
	flushes int
}

func (stub *bridgeLegacyTTSStub) OpenStream(ctx context.Context, opts ...texttospeech.TextToSpeechOption) error {
	_ = ctx

	stub.mu.Lock()
	defer stub.mu.Unlock()

	stub.config = texttospeech.TextToSpeechOptions{}
	for _, opt := range opts {
		opt(&stub.config)
	}
	stub.closed = false
	stub.flushes = 0
	stub.pending.Reset()

	return nil
}

func (stub *bridgeLegacyTTSStub) SendText(text string) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	if stub.closed {
		return fmt.Errorf("legacy tts stream already closed")
	}

	stub.pending.WriteString(text)
	if stub.config.SpeechAudioCallback != nil {
		stub.config.SpeechAudioCallback([]byte(text))
	}

	return nil
}

func (stub *bridgeLegacyTTSStub) FlushBuffer() error {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	if stub.closed {
		return fmt.Errorf("legacy tts stream already closed")
	}

	if stub.config.SpeechMarkCallback != nil {
		stub.config.SpeechMarkCallback(stub.pending.String())
	}
	stub.pending.Reset()
	stub.flushes++

	return nil
}

func (stub *bridgeLegacyTTSStub) Close(context.Context) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.closed = true
	return nil
}

func (stub *bridgeLegacyTTSStub) flushCount() int {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return stub.flushes
}

func (stub *bridgeSpeechGeneratorStub) SendText(text string) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	if stub.closed {
		return fmt.Errorf("generator already closed")
	}

	stub.pending.WriteString(text)
	if stub.config.SpeechAudioCallback != nil {
		stub.config.SpeechAudioCallback([]byte(text))
	}

	return nil
}

func (stub *bridgeSpeechGeneratorStub) Mark() error {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	if stub.closed {
		return fmt.Errorf("generator already closed")
	}

	if stub.config.SpeechMarkCallback != nil {
		stub.config.SpeechMarkCallback(stub.pending.String())
	}
	stub.pending.Reset()

	return nil
}

func (stub *bridgeSpeechGeneratorStub) EndOfText() error {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	if stub.closed {
		return fmt.Errorf("generator already closed")
	}

	if stub.pending.Len() > 0 && stub.config.SpeechMarkCallback != nil {
		stub.config.SpeechMarkCallback(stub.pending.String())
		stub.pending.Reset()
	}
	if stub.config.SpeechEndedCallbackV0 != nil {
		stub.config.SpeechEndedCallbackV0(texttospeech.SpeechEndedReport{})
	}

	return nil
}

func (stub *bridgeSpeechGeneratorStub) Cancel() error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.closed = true
	return nil
}

func (stub *bridgeSpeechGeneratorStub) Close() error {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	stub.closed = true
	return nil
}

type bridgeAudioOutputStub struct {
	mu         sync.Mutex
	audio      [][]byte
	markCount  int
	clearCount int
}

func (output *bridgeAudioOutputStub) EncodingInfo() audio.EncodingInfo {
	return audio.GetDefaultEncodingInfo()
}

func (output *bridgeAudioOutputStub) SendAudio(audio []byte) error {
	output.mu.Lock()
	defer output.mu.Unlock()
	output.audio = append(output.audio, append([]byte(nil), audio...))
	return nil
}

func (output *bridgeAudioOutputStub) ClearBuffer() {
	output.mu.Lock()
	defer output.mu.Unlock()
	output.clearCount++
}

func (output *bridgeAudioOutputStub) Mark(mark string, callback func(string)) error {
	output.mu.Lock()
	output.markCount++
	output.mu.Unlock()

	callback(mark)
	return nil
}

func (output *bridgeAudioOutputStub) nonEmptyAudioChunks() int {
	output.mu.Lock()
	defer output.mu.Unlock()

	nonEmpty := 0
	for _, chunk := range output.audio {
		if len(chunk) > 0 {
			nonEmpty++
		}
	}

	return nonEmpty
}

func (output *bridgeAudioOutputStub) marks() int {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.markCount
}
