package orchestration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const conversationEventQueueCapacity = 10
const defaultSpeechPlayerSegmentationBoundaries = "?.!"

type responsePipeline struct {
	ctxMu sync.RWMutex
	ctx   context.Context

	llm          llm
	textToSpeech *textToSpeech
	speechPlayer *speechPlayer
	audioOutput  *audioOutput

	onCancel func()

	cancelled atomic.Bool
}

func newResponsePipeline(llm llm, textToSpeech *textToSpeech, speechPlayer *speechPlayer, audioOutput *audioOutput, onCancel func()) *responsePipeline {
	speechPlayerSegmentationBoundaries := ""
	if audioOutput.supportsCallbackMarks {
		speechPlayerSegmentationBoundaries = defaultSpeechPlayerSegmentationBoundaries
	}
	speechPlayer.InitBuffers(audioOutput.EncodingInfo(), speechPlayerSegmentationBoundaries)

	if onCancel == nil {
		onCancel = func() {}
	}

	return &responsePipeline{
		llm:          llm,
		textToSpeech: textToSpeech,
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
		return llms.TurnV1{}, fmt.Errorf("turn processor needs to be set")
	}
	if activeTurn == nil {
		return llms.TurnV1{}, fmt.Errorf("active turn is required")
	}

	p.lockFor(func() { p.ctx = ctx })

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer p.Close()

	err := p.runWorkers(ctx, cancel,
		panicSafeNamedWorker("llm generation", func(ctx context.Context) error { return p.generateLLM(ctx, activeTurn, history) }),
		panicSafeNamedWorker("response text processing", func(ctx context.Context) error { return p.processResponseText(ctx, activeTurn) }),
		panicSafeNamedWorker("speech processing", func(ctx context.Context) error { return p.processSpeech(ctx, activeTurn) }),
	)

	if finaliseErr := panicSafeNamedWorker("active turn finalise",
		func(context.Context) error { activeTurn.Finalise(); return nil },
	)(ctx); finaliseErr != nil {
		err = errors.Join(err, finaliseErr)
	}

	if err != nil {
		return activeTurn.TurnV1, fmt.Errorf("one or more active turn processes failed: %w", err)
	}

	return activeTurn.TurnV1, nil
}

func (p *responsePipeline) runWorkers(ctx context.Context, cancel context.CancelFunc, workers ...workerRun) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(workers))

	wg.Add(len(workers))
	for _, run := range workers {
		run := run
		go func() {
			defer wg.Done()
			err := run(ctx)
			if err != nil {
				cancel()
			}
			errCh <- err
		}()
	}

	wg.Wait()
	close(errCh)

	var workerErr error
	for err := range errCh {
		workerErr = errors.Join(workerErr, err)
	}

	return workerErr
}

func (processor *responsePipeline) generateLLM(ctx context.Context, turn *activeTurn, history []llms.TurnV1) error {
	ctx, span := tracer.Start(ctx, "generate llm")
	defer span.End()

	response, err := processor.llm.generate(ctx, turn.Event, history, processor.speechPlayer.AddTextChunk, func() bool {
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

	processor.speechPlayer.TextComplete()
	return nil
}

func (processor *responsePipeline) processResponseText(
	ctx context.Context,
	turn *activeTurn,
) error {
	done := withContextCancelHook(ctx, processor.speechPlayer.ClearText)
	defer close(done)

	_, span := tracer.Start(ctx, "passing text to tts")
	defer span.End()

	if err := processor.textToSpeech.init(ctx, processor.speechPlayer, processor.audioOutput.EncodingInfo()); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

textLoop:
	for textOrMark := range processor.speechPlayer.TextOrMarks {
		if processor.IsCancelled() {
			break textLoop
		}

		switch textOrMark.Type {
		case textOrMarkTypeText:
			chunk := textOrMark.Text
			turn.finalResponse.TypedMessage += chunk

			if err := processor.textToSpeech.SendText(chunk); err != nil {
				span.RecordError(fmt.Errorf("failed to send text to tts: %w", err))
			}
		case textOrMarkTypeMark:
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
			processor.speechPlayer.StopAudio()
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

	const minSpokenTextUpdateInterval = 10 * time.Millisecond
	const maxSpokenTextUpdateInterval = 250 * time.Millisecond

	clampSpokenTextUpdateInterval := func(interval time.Duration) time.Duration {
		if interval < minSpokenTextUpdateInterval {
			return minSpokenTextUpdateInterval
		}
		if interval > maxSpokenTextUpdateInterval {
			return maxSpokenTextUpdateInterval
		}
		return interval
	}

	go func() {
		nextUpdate := processor.speechPlayer.EmitApproximateSpokenTextFromAudioProgressAndNextUpdate()

		timer := time.NewTimer(clampSpokenTextUpdateInterval(nextUpdate))
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-timer.C:
				nextUpdate = processor.speechPlayer.EmitApproximateSpokenTextFromAudioProgressAndNextUpdate()
				timer.Reset(clampSpokenTextUpdateInterval(nextUpdate))
			}
		}
	}()

bufferReadingLoop:
	for audioOrMark := range processor.speechPlayer.Audio {
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
				if transcript := processor.speechPlayer.OnAudioOutputMarkPlayed(mark); transcript != nil {
					turn.finalResponse.SpokenResponse += *transcript
				}
			})
		}
	}

	processor.speechPlayer.OnAudioEnded(processor.speechPlayer.FullText())
	processor.audioOutput.SendAudio([]byte{})
	processor.audioOutput.Clear()

	return nil
}

func (p *responsePipeline) Pause() {
	if p != nil {
		p.speechPlayer.PauseAudio()
		p.audioOutput.Clear()
	}
}

func (p *responsePipeline) Unpause() {
	if p != nil {
		p.speechPlayer.ResumeAudio()
	}
}

func (p *responsePipeline) StopSpeaking() {
	if p != nil {
		p.textToSpeech.Mute()
		p.speechPlayer.StopAudioAndUnblock()
		p.audioOutput.Clear()
	}
}

func (p *responsePipeline) Cancel() {
	if p != nil && p.cancelled.CompareAndSwap(false, true) {
		p.Close()
		p.textToSpeech.Cancel()
		p.speechPlayer.StopAudio()
		p.audioOutput.Clear()
		p.onCancel()
	}
}

func (p *responsePipeline) IsCancelled() bool {
	return p != nil && p.cancelled.Load()
}

func (p *responsePipeline) Close() {
	if p != nil {
		pipelineCtx := p.Ctx()
		if err := p.textToSpeech.Close(pipelineCtx); err != nil {
			err = fmt.Errorf("failed to close tts resources while cancelling active turn: %w", err)
			span := trace.SpanFromContext(pipelineCtx)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
	}
}

func (p *responsePipeline) Ctx() context.Context {
	var ctx context.Context
	p.rLockFor(func() { ctx = p.ctx })
	return ctx
}

func (p *responsePipeline) lockFor(f func()) {
	if p != nil {
		p.ctxMu.Lock()
		defer p.ctxMu.Unlock()
		f()
	}
}

func (p *responsePipeline) rLockFor(f func()) {
	if p != nil {
		p.ctxMu.RLock()
		defer p.ctxMu.RUnlock()
		f()
	}
}
