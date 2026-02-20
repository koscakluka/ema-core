package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const conversationEventQueueCapacity = 10

type responsePipeline struct {
	ctxMu sync.RWMutex
	ctx   context.Context

	llm          llm
	textBuffer   *textBuffer
	textToSpeech *textToSpeech
	audioBuffer  *audioBuffer
	speechPlayer *speechPlayer
	audioOutput  *audioOutput

	onCancel func()

	cancelled atomic.Bool
}

func newResponsePipeline(llm llm, textToSpeech *textToSpeech, speechPlayer *speechPlayer, audioOutput *audioOutput, onCancel func()) *responsePipeline {
	return &responsePipeline{
		llm:          llm,
		textBuffer:   newTextBuffer(),
		textToSpeech: textToSpeech,
		audioBuffer:  newAudioBuffer(audioOutput.EncodingInfo()),
		audioOutput:  audioOutput,
		speechPlayer: speechPlayer,

		onCancel: onCancel,
	}
}

func (p *responsePipeline) Run(
	ctx context.Context,
	activeTurn *activeTurn,
	history []llms.TurnV1,
) (llms.TurnV1, error) {
	if p == nil {
		return llms.TurnV1{}, fmt.Errorf("turn processor and conversation are required")
	}
	if activeTurn == nil {
		return llms.TurnV1{}, fmt.Errorf("active turn is required")
	}

	p.ctxMu.Lock()
	p.ctx = ctx
	p.ctxMu.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var workerErr error
	workerErrMu := sync.Mutex{}
	addWorkerErr := func(err error) {
		if err == nil {
			return
		}
		workerErrMu.Lock()
		workerErr = errors.Join(workerErr, err)
		workerErrMu.Unlock()
	}

	run := func(name string, f func(context.Context) error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				addWorkerErr(fmt.Errorf("%s worker panicked: %v", name, recovered))
				cancel()
			}
		}()

		if err := f(ctx); err != nil {
			addWorkerErr(fmt.Errorf("%s worker failed: %w", name, err))
			cancel()
		}
	}

	wg := &sync.WaitGroup{}
	wg.Add(3)
	go func() {
		defer wg.Done()
		run("llm generation", func(ctx context.Context) error {
			return p.generateLLM(ctx, activeTurn, history)
		})
	}()
	go func() {
		defer wg.Done()
		run("response text processing", func(ctx context.Context) error {
			return p.processResponseText(ctx, activeTurn)
		})
	}()
	go func() {
		defer wg.Done()
		run("speech processing", func(ctx context.Context) error {
			return p.processSpeech(ctx, activeTurn)
		})
	}()

	wg.Wait()

	finaliseErr := func() (err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				err = fmt.Errorf("active turn finalise panicked: %v", recovered)
			}
		}()

		activeTurn.Finalise()
		p.Close()
		return nil
	}()
	addWorkerErr(finaliseErr)

	if workerErr != nil {
		return activeTurn.TurnV1, fmt.Errorf("one or more active turn processes failed: %w", workerErr)
	}

	return activeTurn.TurnV1, nil
}

func (processor *responsePipeline) generateLLM(ctx context.Context, turn *activeTurn, history []llms.TurnV1) error {
	ctx, span := tracer.Start(ctx, "generate llm")
	defer span.End()

	response, err := processor.llm.generate(ctx, turn.Event, history, processor.textBuffer, func() bool {
		return processor.IsCancelled()
	})
	if err != nil {
		err := fmt.Errorf("failed to generate llm response: %w", err)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if response != nil {
		turn.finalResponse.IsMessageFullyGenerated = true
		turn.finalResponse.Message = response.Content
		turn.ToolCalls = response.ToolCalls
		var toolCalls []string
		for _, toolCall := range response.ToolCalls {
			toolCalls = append(toolCalls, toolCall.Name)
		}
		span.SetAttributes(attribute.StringSlice("assistant_turn.tool_calls", toolCalls))
	}

	processor.textBuffer.TextComplete()
	return nil
}

func (processor *responsePipeline) processResponseText(
	ctx context.Context,
	turn *activeTurn,
) error {
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			processor.textBuffer.Clear()
		case <-done:
		}
	}()

	_, span := tracer.Start(ctx, "passing text to tts")
	defer span.End()

	if err := processor.textToSpeech.init(ctx, processor); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

textLoop:
	for chunk := range processor.textBuffer.Chunks {
		if processor.IsCancelled() {
			break textLoop
		}
		turn.finalResponse.TypedMessage += chunk

		if err := processor.textToSpeech.SendText(chunk); err != nil {
			span.RecordError(fmt.Errorf("failed to send text to tts: %w", err))
		}
		if processor.audioOutput.supportsCallbackMarks && strings.ContainsAny(chunk, ".?!") {
			if err := processor.textToSpeech.Mark(); err != nil {
				span.RecordError(fmt.Errorf("failed to send mark to tts: %w", err))
			}
		}
	}

	if err := processor.textToSpeech.EndOfText(); err != nil {
		span.RecordError(fmt.Errorf("failed to end of text to tts: %w", err))
	}

	return nil
}

func (processor *responsePipeline) processSpeech(
	ctx context.Context,
	turn *activeTurn,
) error {
	done := make(chan struct{})
	defer close(done)

	go func() {
		select {
		case <-ctx.Done():
			processor.audioBuffer.Stop()
		case <-done:
		}
	}()

	if ok := processor.textToSpeech.waitUntilInitialized(ctx); !ok {
		return nil
	}
	if !processor.textToSpeech.IsConnected() {
		return nil
	}

	_, span := tracer.Start(ctx, "passing speech to audio output")
	defer span.End()

bufferReadingLoop:
	for audioOrMark := range processor.audioBuffer.Audio {
		switch audioOrMark.Type {
		case "audio":
			audio := audioOrMark.Audio

			if processor.textToSpeech.IsMuted() || processor.IsCancelled() {
				processor.audioOutput.Clear()
				break bufferReadingLoop
			}

			processor.audioOutput.SendAudio(audio)

		case "mark":
			mark := audioOrMark.Mark
			span.AddEvent("received mark", trace.WithAttributes(attribute.String("mark", mark), attribute.String("audio_output.version", "v1")))
			processor.audioOutput.Mark(mark, func(mark string) {
				span.AddEvent("mark played", trace.WithAttributes(attribute.String("mark", mark), attribute.String("audio_output.version", "v1")))
				if transcript := processor.audioBuffer.GetMarkText(mark); transcript != nil {
					turn.finalResponse.SpokenResponse += *transcript
				}
				processor.audioBuffer.ConfirmMark(mark)
			})
		}
	}

	processor.speechPlayer.OnAudioEnded(processor.textBuffer.String())
	processor.audioOutput.SendAudio([]byte{})
	processor.audioOutput.Clear()

	return nil
}

func (p *responsePipeline) Pause() {
	if p == nil {
		return
	}

	p.audioBuffer.Pause()
	p.audioOutput.Clear()
}

func (p *responsePipeline) Unpause() {
	if p == nil {
		return
	}

	p.audioBuffer.Resume()
}

func (p *responsePipeline) StopSpeaking() {
	if p == nil {
		return
	}

	p.textToSpeech.Mute()
	p.audioBuffer.AddAudio([]byte{})
	p.audioBuffer.Stop()
	p.audioOutput.Clear()
}

func (p *responsePipeline) Cancel() {
	if p == nil || !p.cancelled.CompareAndSwap(false, true) {
		return
	}

	p.Close()
	p.textToSpeech.Cancel()
	p.audioBuffer.Stop()
	p.audioOutput.Clear()
	if p.onCancel != nil {
		p.onCancel()
	}
}

func (p *responsePipeline) IsCancelled() bool {
	if p == nil {
		return false
	}

	return p.cancelled.Load()
}

func (p *responsePipeline) Close() {
	if p == nil {
		return
	}

	pipelineCtx := p.Ctx()
	if err := p.textToSpeech.Close(pipelineCtx); err != nil {
		err = fmt.Errorf("failed to close tts resources while cancelling active turn: %w", err)
		span := trace.SpanFromContext(pipelineCtx)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

func (p *responsePipeline) Ctx() context.Context {
	if p == nil {
		return nil
	}

	p.ctxMu.RLock()
	defer p.ctxMu.RUnlock()

	return p.ctx
}
