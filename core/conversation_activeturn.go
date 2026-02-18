package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type activeTurn struct {
	llms.TurnV1

	ctx         context.Context
	textBuffer  textBuffer
	audioBuffer audioBuffer
	// textToSpeech is the per-turn facade around configured TTS clients.
	textToSpeech textToSpeech
	// audioOutput is the per-turn facade around configured audio output clients.
	audioOutput   audioOutput
	finalResponse *llms.TurnResponseV0

	llm        *llm
	isSpeaking bool

	onResponseText      func(string)
	onResponseTextEnd   func()
	onResponseSpeech    func([]byte)
	onResponseSpeechEnd func(string)
	onCancel            func()

	cancelled atomic.Bool
	err       error
}

func newActiveTurn(
	ctx context.Context,
	event llms.EventV0,
	llm *llm,
	textToSpeechClient textToSpeechBase,
	audioOutputClient audioOutputBase,
	isSpeaking bool,
	onResponseText func(string),
	onResponseTextEnd func(),
	onResponseSpeech func([]byte),
	onResponseSpeechEnd func(string),
	onCancel func(),
) *activeTurn {
	if onResponseText == nil {
		onResponseText = func(string) {}
	}
	if onResponseTextEnd == nil {
		onResponseTextEnd = func() {}
	}
	if onResponseSpeech == nil {
		onResponseSpeech = func([]byte) {}
	}
	if onResponseSpeechEnd == nil {
		onResponseSpeechEnd = func(string) {}
	}
	if onCancel == nil {
		onCancel = func() {}
	}

	audioOutput := newAudioOutput(audioOutputClient)

	return &activeTurn{
		TurnV1: llms.TurnV1{
			ID:    uuid.NewString(),
			Event: event,
		},

		ctx:                 ctx,
		textBuffer:          *newTextBuffer(),
		audioBuffer:         *newAudioBuffer(audioOutput.EncodingInfo()),
		audioOutput:         *audioOutput,
		textToSpeech:        *newTextToSpeech(textToSpeechClient),
		llm:                 llm,
		isSpeaking:          isSpeaking,
		onResponseText:      onResponseText,
		onResponseTextEnd:   onResponseTextEnd,
		onResponseSpeech:    onResponseSpeech,
		onResponseSpeechEnd: onResponseSpeechEnd,
		onCancel:            onCancel,

		finalResponse: &llms.TurnResponseV0{},
	}
}

func (t *activeTurn) AddInterruption(interruption llms.InterruptionV0) error {
	if t.IsCancelled() {
		return fmt.Errorf("turn cancelled")
	} else if t.IsFinalised {
		return fmt.Errorf("turn already finalized")
	}

	t.Interruptions = append(t.Interruptions, interruption)
	return nil
}

func (t *activeTurn) StopSpeaking() {
	if !t.isSpeaking {
		return
	}

	t.isSpeaking = false
	t.audioBuffer.AddAudio([]byte{})
	t.audioBuffer.Stop()
	t.audioOutput.Clear()
}

func (t *activeTurn) Pause() {
	t.audioBuffer.Pause()
	t.audioOutput.Clear()
}

func (t *activeTurn) Unpause() {
	t.audioBuffer.Resume()
}

func (t *activeTurn) Finalise() {
	if t.IsFinalised {
		return
	}

	if t.finalResponse != nil {
		t.Responses = append(t.Responses, *t.finalResponse)
	}
	t.IsFinalised = true
	if err := t.textToSpeech.Close(t.ctx); err != nil {
		err = fmt.Errorf("failed to close tts resources while finalising active turn: %w", err)
		span := trace.SpanFromContext(t.ctx)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

func (t *activeTurn) Cancel() {
	if !t.cancelled.CompareAndSwap(false, true) {
		return
	}
	t.textBuffer.Clear()
	if err := t.textToSpeech.Cancel(); err != nil {
		err = fmt.Errorf("failed to cancel tts resources while cancelling active turn: %w", err)
		span := trace.SpanFromContext(t.ctx)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	t.audioBuffer.Stop()
	t.audioOutput.Clear()
	t.onCancel()
}

func (t *activeTurn) IsCancelled() bool {
	return t.cancelled.Load()
}

func (t *activeTurn) IsMutable() bool {
	return !t.IsFinalised
}

func (t *activeTurn) generateLLM(ctx context.Context, conversation []llms.TurnV1) error {
	ctx, span := tracer.Start(ctx, "generate llm")
	defer span.End()

	if t.llm == nil {
		t.textBuffer.TextComplete()
		return nil
	}

	response, err := t.llm.generate(ctx, t.Event, conversation, &t.textBuffer, func() bool {
		return t.IsCancelled()
	})
	if err != nil {
		err := fmt.Errorf("failed to generate llm response: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		t.err = errors.Join(t.err, err)
		return err
	}
	if response != nil {
		t.finalResponse.IsMessageFullyGenerated = true
		t.finalResponse.Message = response.Content
		t.ToolCalls = response.ToolCalls
		var toolCalls []string
		for _, toolCall := range response.ToolCalls {
			toolCalls = append(toolCalls, toolCall.Name)
		}
		span.SetAttributes(attribute.StringSlice("assistant_turn.tool_calls", toolCalls))
	}

	t.textBuffer.TextComplete()
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

	if err := t.textToSpeech.init(ctx, t); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

textLoop:
	for chunk := range t.textBuffer.Chunks {
		if t.IsCancelled() {
			break textLoop
		}
		t.finalResponse.TypedMessage += chunk
		t.onResponseText(chunk)

		if err := t.textToSpeech.SendText(chunk); err != nil {
			span.RecordError(fmt.Errorf("failed to send text to tts: %w", err))
		}
		if t.audioOutput.supportsCallbackMarks && strings.ContainsAny(chunk, ".?!") {
			if err := t.textToSpeech.Mark(); err != nil {
				span.RecordError(fmt.Errorf("failed to send mark to tts: %w", err))
			}
		}
	}

	if err := t.textToSpeech.EndOfText(); err != nil {
		span.RecordError(fmt.Errorf("failed to end of text to tts: %w", err))
	}

	t.onResponseTextEnd()
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

	t.textToSpeech.waitUntilInitialized()
	if !t.textToSpeech.connected {
		return nil
	}

	_, span := tracer.Start(ctx, "passing speech to audio output")
	defer span.End()

bufferReadingLoop:
	for audioOrMark := range t.audioBuffer.Audio {
		switch audioOrMark.Type {
		case "audio":
			audio := audioOrMark.Audio
			t.onResponseSpeech(audio)

			if !t.isSpeaking || t.IsCancelled() {
				t.audioOutput.Clear()
				break bufferReadingLoop
			}

			t.audioOutput.SendAudio(audio)

		case "mark":
			mark := audioOrMark.Mark
			span.AddEvent("received mark", trace.WithAttributes(attribute.String("mark", mark), attribute.String("audio_output.version", "v1")))
			t.audioOutput.Mark(mark, func(mark string) {
				span.AddEvent("mark played", trace.WithAttributes(attribute.String("mark", mark), attribute.String("audio_output.version", "v1")))
				if transcript := t.audioBuffer.GetMarkText(mark); transcript != nil {
					t.finalResponse.SpokenResponse += *transcript
				}
				t.audioBuffer.ConfirmMark(mark)
			})
		}
	}

	t.onResponseSpeechEnd(t.textBuffer.String())
	// TODO: Figure out why sendaudio is needed
	t.audioOutput.SendAudio([]byte{})
	t.audioOutput.Clear()

	return nil
}
