package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/core/texttospeech"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type activeTurn struct {
	llms.Turn

	ctx         context.Context
	textBuffer  textBuffer
	audioBuffer audioBuffer
	tts         activeTurnTTS         // NOTE: tmp until we can remove the old TTS version
	audioOut    activeTurnAudioOutput // NOTE: tmp until we can remove the old audioOutput version

	components activeTurnComponents
	callbacks  activeTurnCallbacks
	config     activeTurnConfig

	err error
}

type activeTurnComponents struct {
	TextToSpeechClient textToSpeech
	AudioOutput        audioOutput
	ResponseGenerator  func(context.Context, *textBuffer) (*llms.Turn, error) // TODO: Fix the signature to include prompt and "history"
}

type activeTurnCallbacks struct {
	OnResponseText      func(string)
	OnResponseTextEnd   func()
	OnResponseSpeech    func([]byte)
	OnResponseSpeechEnd func(string)
	OnFinalise          func(*activeTurn)
}

func (c *activeTurnCallbacks) defaults() *activeTurnCallbacks {
	c.OnResponseText = func(response string) {}
	c.OnResponseTextEnd = func() {}
	c.OnResponseSpeech = func([]byte) {}
	c.OnResponseSpeechEnd = func(string) {}
	c.OnFinalise = func(*activeTurn) {}
	return c
}

func (c *activeTurnCallbacks) with(callbacks activeTurnCallbacks) *activeTurnCallbacks {
	if callbacks.OnResponseText != nil {
		c.OnResponseText = callbacks.OnResponseText
	}
	if callbacks.OnResponseTextEnd != nil {
		c.OnResponseTextEnd = callbacks.OnResponseTextEnd
	}
	if callbacks.OnFinalise != nil {
		c.OnFinalise = callbacks.OnFinalise
	}
	if callbacks.OnResponseSpeech != nil {
		c.OnResponseSpeech = callbacks.OnResponseSpeech
	}
	if callbacks.OnResponseSpeechEnd != nil {
		c.OnResponseSpeechEnd = callbacks.OnResponseSpeechEnd
	}
	return c
}

type activeTurnConfig struct {
	IsSpeaking bool
}

func newActiveTurn(ctx context.Context, components activeTurnComponents, callbacks activeTurnCallbacks, config activeTurnConfig) *activeTurn {
	activeTurn := &activeTurn{
		Turn: llms.Turn{Role: llms.TurnRoleAssistant},

		ctx:         ctx,
		textBuffer:  *newTextBuffer(),
		audioBuffer: *newAudioBuffer(),
		tts:         activeTurnTTS{},
		components:  components,
		callbacks:   *(new(activeTurnCallbacks).defaults().with(callbacks)),
		config:      config,
	}

	if activeTurn.components.AudioOutput != nil {
		activeTurn.audioBuffer.sampleRate = activeTurn.components.AudioOutput.EncodingInfo().SampleRate
	}
	activeTurn.audioOut.init(activeTurn)

	return activeTurn
}

func (t *activeTurn) AddInterruption(interruption llms.InterruptionV0) error {
	if t.Cancelled {
		return fmt.Errorf("turn cancelled")
	} else if t.Stage == llms.TurnStageFinalized {
		return fmt.Errorf("turn already finalized")
	}

	t.Interruptions = append(t.Interruptions, interruption)
	return nil
}

func (t *activeTurn) StopSpeaking() {
	t.config.IsSpeaking = false
	t.audioBuffer.AddAudio([]byte{})
}

func (t *activeTurn) Pause() {
	t.audioBuffer.Pause()
}

func (t *activeTurn) Unpause() {
	t.audioBuffer.Resume()
}

func (t *activeTurn) Finalise() {
	if t.Stage == llms.TurnStageFinalized {
		return
	}

	t.Turn.Stage = llms.TurnStageFinalized
	t.tts.Close(t.ctx)
	t.callbacks.OnFinalise(t)
}

func (t *activeTurn) IsMutable() bool {
	return !t.IsFinalized()
}

func (t *activeTurn) IsFinalized() bool {
	return t.Stage == llms.TurnStageFinalized || t.Cancelled
}

func (t *activeTurn) generateResponse(ctx context.Context) error {
	ctx, span := tracer.Start(ctx, "generate response")
	defer span.End()

	response, err := t.components.ResponseGenerator(ctx, &t.textBuffer)
	if err != nil {
		err := fmt.Errorf("failed to generate response: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		t.err = errors.Join(t.err, err)
		return err
	}
	if response != nil {
		var toolCalls []string
		for _, toolCall := range response.ToolCalls {
			toolCalls = append(toolCalls, toolCall.Name)
		}
		span.SetAttributes(attribute.StringSlice("assistant_turn.tool_calls", toolCalls))
	}

	t.textBuffer.TextComplete()
	if t.IsMutable() {
		t.Content = response.Content
		t.ToolCalls = response.ToolCalls
	}

	return nil
}

func (t *activeTurn) processResponseText(ctx context.Context) error {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			t.textBuffer.Clear()
		case <-done:
		}
	}()

	_, span := tracer.Start(ctx, "passing text to tts")
	defer span.End()

	if err := t.tts.init(ctx, t); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

textLoop:
	for chunk := range t.textBuffer.Chunks {
		if t.Cancelled {
			break textLoop
		}
		t.callbacks.OnResponseText(chunk)

		if err := t.tts.SendText(chunk); err != nil {
			span.RecordError(fmt.Errorf("failed to send text to tts: %w", err))
		}
		if t.audioOut.supportsCallbackMarks && strings.ContainsAny(chunk, ".?!") {
			if err := t.tts.Mark(); err != nil {
				span.RecordError(fmt.Errorf("failed to send mark to tts: %w", err))
			}
		}
	}

	if err := t.tts.EndOfText(); err != nil {
		span.RecordError(fmt.Errorf("failed to end of text to tts: %w", err))
	}

	t.callbacks.OnResponseTextEnd()
	return nil
}

func (t *activeTurn) processSpeech(ctx context.Context) error {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			t.audioBuffer.Stop()
		case <-done:
		}
	}()

	t.tts.waitUntilInitialized()
	if !t.tts.connected {
		return nil
	}

	_, span := tracer.Start(ctx, "passing speech to audio output")
	defer span.End()

bufferReadingLoop:
	for audioOrMark := range t.audioBuffer.Audio {
		switch audioOrMark.Type {
		case "audio":
			audio := audioOrMark.Audio
			t.callbacks.OnResponseSpeech(audio)

			if !t.config.IsSpeaking || t.Cancelled {
				t.audioOut.Clear()
				break bufferReadingLoop
			}

			t.audioOut.SendAudio(audio)

		case "mark":
			mark := audioOrMark.Mark
			span.AddEvent("received mark", trace.WithAttributes(attribute.String("mark", mark), attribute.String("audio_output.version", "v1")))
			t.audioOut.Mark(mark, func(mark string) {
				span.AddEvent("mark played", trace.WithAttributes(attribute.String("mark", mark), attribute.String("audio_output.version", "v1")))
				t.audioBuffer.ConfirmMark(mark)
			})
		}
	}

	t.callbacks.OnResponseSpeechEnd(t.textBuffer.String())
	// TODO: Figure out why sendaudio is needed
	t.audioOut.SendAudio([]byte{})
	t.audioOut.Clear()

	return nil
}

// NOTE: Helpers after this point are temporary and will be removed or replaced
// once the the API stabilizes

// activeTurnTTS is a temporary type to allow a simple interface for different
// TTS versions
//
// DO NOT USE! This is a temporary type and will be removed once we can remove
// the old TTS version
type activeTurnTTS struct {
	ttsClient    TextToSpeech
	ttsGenerator texttospeech.SpeechGeneratorV0

	initialized   chan struct{}
	connected     bool
	clientStarted bool
}

// init is a temporary method that setups up different TTS versions
//
// DO NOT USE! This is a temporary method and will be removed once we can remove
// the old TTS version
func (t *activeTurnTTS) init(ctx context.Context, turn *activeTurn) error {
	if t.initialized == nil {
		t.initialized = make(chan struct{})
	}
	defer close(t.initialized)

	ttsOptions := []texttospeech.TextToSpeechOption{
		texttospeech.WithSpeechAudioCallback(turn.audioBuffer.AddAudio),
		texttospeech.WithSpeechMarkCallback(turn.audioBuffer.Mark),
	}
	if turn.components.AudioOutput != nil {
		ttsOptions = append(ttsOptions, texttospeech.WithEncodingInfo(turn.components.AudioOutput.EncodingInfo()))
	}

	if turn.components.TextToSpeechClient != nil {
		if client, ok := turn.components.TextToSpeechClient.(TextToSpeechV1); ok {
			ttsOptions = append(ttsOptions, texttospeech.WithSpeechEndedCallbackV0(func(report texttospeech.SpeechEndedReport) {
				// TODO: See if we need to do something smarter here
				turn.audioBuffer.AllAudioLoaded()
			}))

			speechGenerator, err := client.NewSpeechGeneratorV0(ctx, ttsOptions...)
			if err != nil {
				return fmt.Errorf("failed to create speech generator: %w", err)
			}
			t.ttsGenerator = speechGenerator
			t.connected = true
		} else if client, ok := turn.components.TextToSpeechClient.(TextToSpeech); ok {
			if err := client.OpenStream(ctx, ttsOptions...); err != nil {
				return fmt.Errorf("failed to open tts stream: %w", err)
			}
			t.ttsClient = client
			t.connected = true
			turn.audioBuffer.usingWithLegacyTTS = true
		}
	}
	return nil
}

func (t *activeTurnTTS) waitUntilInitialized() {
	for t.initialized == nil {
		time.Sleep(time.Millisecond * 10)
	}
	<-t.initialized
}

func (t *activeTurnTTS) Close(ctx context.Context) error {
	if t.ttsClient != nil && t.clientStarted {
		if client, ok := t.ttsClient.(interface{ Close(ctx context.Context) }); ok {
			client.Close(ctx)
		}
	} else if t.ttsGenerator != nil {
		if err := t.ttsGenerator.Close(); err != nil {
			return fmt.Errorf("failed to close tts: %w", err)
		}
	}
	return nil
}

func (t *activeTurnTTS) SendText(text string) error {
	if t.ttsClient != nil {
		if err := t.ttsClient.SendText(text); err != nil {
			return fmt.Errorf("failed to send text to tts: %w", err)
		}
		if t.ttsGenerator != nil {
			if err := t.ttsGenerator.SendText(text); err != nil {
				return fmt.Errorf("failed to send text to tts: %w", err)
			}
		}
	} else if t.ttsGenerator != nil {
		if err := t.ttsGenerator.SendText(text); err != nil {
			return fmt.Errorf("failed to send text to tts: %w", err)
		}
	}

	return nil
}

func (t *activeTurnTTS) Mark() error {
	if t.ttsClient != nil {
		if err := t.ttsClient.FlushBuffer(); err != nil {
			return fmt.Errorf("failed to send flush to tts: %w", err)
		}
	} else if t.ttsGenerator != nil {
		if err := t.ttsGenerator.Mark(); err != nil {
			return fmt.Errorf("failed to send mark to tts: %w", err)
		}
	}

	return nil
}

func (t *activeTurnTTS) EndOfText() error {
	if t.ttsClient != nil {
		if err := t.ttsClient.FlushBuffer(); err != nil {
			return fmt.Errorf("failed to send flush to tts: %w", err)
		}
	} else if t.ttsGenerator != nil {
		if err := t.ttsGenerator.Mark(); err != nil {
			return fmt.Errorf("failed to send flush to tts: %w", err)
		}
		if err := t.ttsGenerator.EndOfText(); err != nil {
			return fmt.Errorf("failed to send end of text to tts: %w", err)
		}
	}
	return nil
}

// activeTurnAudioOutput is a temporary type to allow a simple interface for
// different audio output versions
//
// DO NOT USE! This is a temporary type and will be removed once we can remove
// the old audio output version
type activeTurnAudioOutput struct {
	v0 AudioOutputV0
	v1 AudioOutputV1

	connected             bool
	supportsCallbackMarks bool
	isV1                  bool
}

// init is a temporary method that setups up different audio output versions
//
// DO NOT USE! This is a temporary method and will be removed once we can remove
// the old audio output version
func (t *activeTurnAudioOutput) init(turn *activeTurn) {
	if turn.components.AudioOutput != nil {
		if _, ok := turn.components.AudioOutput.(AudioOutputV1); ok {
			t.v1 = turn.components.AudioOutput.(AudioOutputV1)
			t.isV1 = true
			t.supportsCallbackMarks = true
			t.connected = true
		} else if client, ok := turn.components.AudioOutput.(AudioOutputV0); ok {
			t.v0 = client
			t.connected = true
		}
	}
}

func (t *activeTurnAudioOutput) SendAudio(audio []byte) {
	if t.v1 != nil {
		t.v1.SendAudio(audio)
	} else if t.v0 != nil {
		t.v0.SendAudio(audio)
	}
}

func (t *activeTurnAudioOutput) Mark(mark string, callback func(string)) {
	if t.v1 != nil {
		t.v1.Mark(mark, callback)
	} else if t.v0 != nil {
		go func() {
			t.v0.AwaitMark()
			callback(mark)
		}()
	} else {
		callback(mark)
	}
}

func (t *activeTurnAudioOutput) Clear() {
	if t.v1 != nil {
		t.v1.ClearBuffer()
	} else if t.v0 != nil {
		t.v0.ClearBuffer()
	}
}
