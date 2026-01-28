package orchestration

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Turns struct {
	turns []llms.Turn
	// TODO: Consider adding ID to turns to be able to find the active turn
	// if needed instead of keeping track of an index

	activeTurn *activeTurn
}

// Push adds a new turn to the stored turns
func (t *Turns) Push(turn llms.Turn) {
	t.turns = append(t.turns, turn)
}

// Pop removes the last turn from the stored turns, returns nil if empty
func (t *Turns) Pop() *llms.Turn {
	if activeTurn := t.activeTurn; activeTurn != nil {
		activeTurn.Cancelled = true
		t.activeTurn = nil
		return &activeTurn.Turn
	}

	if len(t.turns) == 0 {
		return nil
	}
	lastElementIdx := len(t.turns) - 1
	turn := t.turns[lastElementIdx]
	t.turns = t.turns[:lastElementIdx]
	return &turn
}

// Clear removes all stored turns
func (t *Turns) Clear() {
	t.turns = nil
	t.activeTurn = nil
}

// Values is an iterator that goes over all the stored turns starting from the
// earliest towards the latest
func (t *Turns) Values(yield func(llms.Turn) bool) {
	for _, turn := range t.turns {
		if !yield(turn) {
			return
		}
	}
	if activeTurn := t.activeTurn; activeTurn != nil {
		if !yield(activeTurn.Turn) {
			return
		}
	}
}

// Values is an iterator that goes over all the stored turns starting from the
// latest towards the earliest
func (t *Turns) RValues(yield func(llms.Turn) bool) {
	if activeTurn := t.activeTurn; activeTurn != nil {
		if !yield(activeTurn.Turn) {
			return
		}
	}
	// TODO: There should be a better way to do this than creating a new
	// method just for reversing the order
	for _, turn := range slices.Backward(t.turns) {
		if !yield(turn) {
			return
		}
	}
}

func (t *Turns) startActiveTurn(ctx context.Context, components activeTurnComponents, callbacks activeTurnCallbacks, config activeTurnConfig) error {
	// TODO: active turn needs a mutex (not really but it would be nice)
	if t.activeTurn != nil {
		return fmt.Errorf("active turn already set")
	}

	t.activeTurn = &activeTurn{
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
		t.activeTurn.callbacks.OnResponseText = callbacks.OnResponseText
	}
	if callbacks.OnResponseTextEnd != nil {
		t.activeTurn.callbacks.OnResponseTextEnd = callbacks.OnResponseTextEnd
	}
	if callbacks.OnFinalise != nil {
		t.activeTurn.callbacks.OnFinalise = callbacks.OnFinalise
	}
	if callbacks.OnResponseSpeech != nil {
		t.activeTurn.callbacks.OnResponseSpeech = callbacks.OnResponseSpeech
	}
	if callbacks.OnResponseSpeechEnd != nil {
		t.activeTurn.callbacks.OnResponseSpeechEnd = callbacks.OnResponseSpeechEnd
	}

	if t.activeTurn.components.AudioOutput != nil {
		t.activeTurn.audioBuffer.sampleRate = t.activeTurn.components.AudioOutput.EncodingInfo().SampleRate
	}

	go t.activeTurn.processResponseText()
	go t.activeTurn.processSpeech()

	t.activeTurn.generateResponse()

	return nil
}

func (t *Turns) updateInterruption(id int64, update func(*llms.InterruptionV0)) {
	if t.activeTurn != nil {
		for j, interruption := range t.activeTurn.Interruptions {
			if interruption.ID == id {
				update(&t.activeTurn.Interruptions[j])
				return
			}
		}
	}
	for i, turn := range slices.Backward(t.turns) {
		for j, interruption := range turn.Interruptions {
			if interruption.ID == id {
				update(&t.turns[i].Interruptions[j])
			}
		}
	}
}

type activeTurn struct {
	llms.Turn

	ctx         context.Context
	textBuffer  textBuffer
	audioBuffer audioBuffer

	components activeTurnComponents
	callbacks  activeTurnCallbacks
	config     activeTurnConfig
}

type activeTurnComponents struct {
	TextToSpeechClient TextToSpeech
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

type activeTurnConfig struct {
	IsSpeaking bool
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

func (t *activeTurn) Finalise() {
	t.Turn.Stage = llms.TurnStageFinalized
	t.callbacks.OnFinalise(t)
}

func (t *activeTurn) IsMutable() bool {
	return !t.IsFinalized()
}

func (t *activeTurn) IsFinalized() bool {
	return t.Stage == llms.TurnStageFinalized || t.Cancelled
}

func (t *activeTurn) generateResponse() {
	ctx, span := tracer.Start(t.ctx, "generate response")
	defer span.End()
	response, err := t.components.ResponseGenerator(ctx, &t.textBuffer)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return
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
}

func (t *activeTurn) processResponseText() {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-t.ctx.Done():
			t.textBuffer.Clear()
		case <-done:
		}
	}()

	_, span := tracer.Start(t.ctx, "passing text to tts")
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
		}
	}

	if t.components.TextToSpeechClient != nil {
		if err := t.components.TextToSpeechClient.FlushBuffer(); err != nil {
			span.RecordError(fmt.Errorf("failed to flush buffer: %w", err))
		}
	} else if !t.Cancelled {
		t.Finalise()
	}

	t.callbacks.OnResponseTextEnd()
}

func (t *activeTurn) processSpeech() {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-t.ctx.Done():
			t.audioBuffer.Clear()
		case <-done:
		}
	}()

	_, span := tracer.Start(t.ctx, "passing speech to audio output")
	defer span.End()
bufferReadingLoop:
	// TODO: This can panic if active turn ends up being nil, there should be a
	// way around this, specifically, for the active turn to handle this loop
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
			span.AddEvent("received mark", trace.WithAttributes(attribute.String("mark", mark)))
			if t.components.AudioOutput != nil {
				switch t.components.AudioOutput.(type) {
				case AudioOutputV1:
					t.components.AudioOutput.(AudioOutputV1).Mark(mark, func(mark string) {
						span.AddEvent("mark played", trace.WithAttributes(attribute.String("mark", mark), attribute.String("audio_output.version", "v1")))
						t.audioBuffer.MarkPlayed(mark)
					})
				case AudioOutputV0:
					go func() {
						span.AddEvent("mark played", trace.WithAttributes(attribute.String("mark", mark), attribute.String("audio_output.version", "v0")))
						t.components.AudioOutput.(AudioOutputV0).AwaitMark()
						t.audioBuffer.MarkPlayed(mark)
					}()
				}
			} else {
				span.AddEvent("mark played", trace.WithAttributes(attribute.String("mark", mark), attribute.Bool("audio_output.set", false)))
				t.audioBuffer.MarkPlayed(mark)
			}
		}
	}

	defer func() { t.Finalise() }()

	t.callbacks.OnResponseSpeechEnd(t.audioBuffer.audioTranscript)

	if t.components.AudioOutput == nil {
		return
	}

	// TODO: Figure out why this is needed
	t.components.AudioOutput.SendAudio([]byte{})

	if !t.config.IsSpeaking || t.Cancelled {
		t.components.AudioOutput.ClearBuffer()
	}

}
