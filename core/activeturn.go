package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type activeTurn struct {
	llms.Turn

	ctx         context.Context
	textBuffer  textBuffer
	audioBuffer audioBuffer

	components activeTurnComponents
	callbacks  activeTurnCallbacks
	config     activeTurnConfig

	err error
}

type activeTurnComponents struct {
	TextToSpeechClient    TextToSpeech
	TextToSpeechGenerator interface {
		SendText(string) error
		Mark() error
		EndOfText() error
		Cancel() error
		Close() error
	}
	AudioOutput       audioOutput
	ResponseGenerator func(context.Context, *textBuffer) (*llms.Turn, error) // TODO: Fix the signature to include prompt and "history"
}

type activeTurnCallbacks struct {
	OnResponseText      func(string)
	OnResponseTextEnd   func()
	OnResponseSpeech    func([]byte)
	OnResponseSpeechEnd func(string)
	OnFinalise          func(*activeTurn)
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
		components:  components,
		callbacks: activeTurnCallbacks{
			OnResponseText:      func(response string) {},
			OnResponseTextEnd:   func() {},
			OnResponseSpeech:    func([]byte) {},
			OnResponseSpeechEnd: func(string) {},
			OnFinalise:          func(*activeTurn) {},
		},
		config: config,
	}

	if callbacks.OnResponseText != nil {
		activeTurn.callbacks.OnResponseText = callbacks.OnResponseText
	}
	if callbacks.OnResponseTextEnd != nil {
		activeTurn.callbacks.OnResponseTextEnd = callbacks.OnResponseTextEnd
	}
	if callbacks.OnFinalise != nil {
		activeTurn.callbacks.OnFinalise = callbacks.OnFinalise
	}
	if callbacks.OnResponseSpeech != nil {
		activeTurn.callbacks.OnResponseSpeech = callbacks.OnResponseSpeech
	}
	if callbacks.OnResponseSpeechEnd != nil {
		activeTurn.callbacks.OnResponseSpeechEnd = callbacks.OnResponseSpeechEnd
	}

	if activeTurn.components.AudioOutput != nil {
		activeTurn.audioBuffer.sampleRate = activeTurn.components.AudioOutput.EncodingInfo().SampleRate
	}

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
	t.audioBuffer.PauseAudio()
}

func (t *activeTurn) Unpause() {
	t.audioBuffer.UnpauseAudio()
}

func (t *activeTurn) finalise() {
	t.Turn.Stage = llms.TurnStageFinalized
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

	t.textBuffer.ChunksDone()
	t.audioBuffer.ChunksDone()
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
textLoop:
	for chunk := range t.textBuffer.Chunks {
		if t.Cancelled {
			break textLoop
		}
		t.callbacks.OnResponseText(chunk)

		if t.components.TextToSpeechClient != nil {
			if err := t.components.TextToSpeechClient.SendText(chunk); err != nil {
				span.RecordError(fmt.Errorf("failed to send text to deepgram: %w", err))
			}
			if t.components.AudioOutput != nil {
				if _, ok := t.components.AudioOutput.(AudioOutputV1); ok {
					if strings.ContainsAny(chunk, ".?!") {
						if err := t.components.TextToSpeechClient.FlushBuffer(); err != nil {
							span.RecordError(fmt.Errorf("failed to flush buffer: %w", err))
						}
					}
				}
			}
		} else if t.components.TextToSpeechGenerator != nil {
			if err := t.components.TextToSpeechGenerator.SendText(chunk); err != nil {
				span.RecordError(fmt.Errorf("failed to send text to deepgram: %w", err))
			}
			if t.components.AudioOutput != nil {
				if _, ok := t.components.AudioOutput.(AudioOutputV1); ok {
					if strings.ContainsAny(chunk, ".?!") {
						if err := t.components.TextToSpeechGenerator.Mark(); err != nil {
							span.RecordError(fmt.Errorf("failed to flush buffer: %w", err))
						}
					}
				}
			}
		}
	}

	if t.components.TextToSpeechClient != nil {
		if err := t.components.TextToSpeechClient.FlushBuffer(); err != nil {
			span.RecordError(fmt.Errorf("failed to flush buffer: %w", err))
		}
	} else if t.components.TextToSpeechGenerator != nil {
		if err := t.components.TextToSpeechGenerator.EndOfText(); err != nil {
			span.RecordError(fmt.Errorf("failed to flush buffer: %w", err))
		}
	} else if !t.Cancelled {
		t.finalise()
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
			t.audioBuffer.Clear()
		case <-done:
		}
	}()

	_, span := tracer.Start(ctx, "passing speech to audio output")
	defer span.End()
bufferReadingLoop:
	for audioOrMark := range t.audioBuffer.Audio {
		switch audioOrMark.Type {
		case "audio":
			audio := audioOrMark.Audio
			t.callbacks.OnResponseSpeech(audio)

			if t.components.AudioOutput == nil {
				continue bufferReadingLoop
			}

			if !t.config.IsSpeaking || t.Cancelled {
				t.components.AudioOutput.ClearBuffer()
				break bufferReadingLoop
			}

			t.components.AudioOutput.SendAudio(audio)

		case "mark":
			mark := audioOrMark.Mark
			if t.components.AudioOutput != nil {
				switch t.components.AudioOutput.(type) {
				case AudioOutputV1:
					span.AddEvent("received mark", trace.WithAttributes(attribute.String("mark", mark), attribute.String("audio_output.version", "v1")))
					t.components.AudioOutput.(AudioOutputV1).Mark(mark, func(mark string) {
						span.AddEvent("mark played", trace.WithAttributes(attribute.String("mark", mark), attribute.String("audio_output.version", "v1")))
						t.audioBuffer.MarkPlayed(mark)
					})
				case AudioOutputV0:
					span.AddEvent("received mark", trace.WithAttributes(attribute.String("mark", mark), attribute.String("audio_output.version", "v0")))
					go func() {
						span.AddEvent("mark played", trace.WithAttributes(attribute.String("mark", mark), attribute.String("audio_output.version", "v0")))
						t.components.AudioOutput.(AudioOutputV0).AwaitMark()
						t.audioBuffer.MarkPlayed(mark)
					}()
				}
			} else {
				span.AddEvent("received mark", trace.WithAttributes(attribute.String("mark", mark), attribute.Bool("audio_output.set", false)))
				span.AddEvent("mark played", trace.WithAttributes(attribute.String("mark", mark), attribute.Bool("audio_output.set", false)))
				t.audioBuffer.MarkPlayed(mark)
			}
		}
	}

	defer func() { t.finalise() }()

	t.callbacks.OnResponseSpeechEnd(t.audioBuffer.audioTranscript)

	if t.components.AudioOutput == nil {
		return nil
	}

	// TODO: Figure out why this is needed
	t.components.AudioOutput.SendAudio([]byte{})
	t.components.AudioOutput.ClearBuffer()

	if !t.config.IsSpeaking || t.Cancelled {
		t.components.AudioOutput.ClearBuffer()
	}

	return nil

}
