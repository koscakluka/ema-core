package orchestration

import (
	"context"
	"fmt"
	"strings"
	"time"

	"log"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func (o *Orchestrator) startAssistantLoop() {
	for promptQueueItem := range o.transcripts {
		if o.turns.activeTurn != nil {
			o.promptEnded.Wait()
		}
		o.promptEnded.Add(1)

		ctx, span := tracer.Start(o.baseContext, "process turn")
		span.AddEvent("taken out of queue", trace.WithAttributes(attribute.Float64("assistant_turn.queued_time", time.Since(promptQueueItem.queuedAt).Seconds())))
		span.SetAttributes(attribute.Float64("assistant_turn.queued_time", time.Since(promptQueueItem.queuedAt).Seconds()))
		transcript := promptQueueItem.content

		messages := o.turns
		o.turns.Push(llms.Turn{
			Role:    llms.TurnRoleUser,
			Content: transcript,
		})

		components := activeTurnComponents{
			AudioOutput: o.audioOutput,
			ResponseGenerator: func(ctx context.Context, buffer *textBuffer) (*llms.Turn, error) {
				switch o.llm.(type) {
				case LLMWithStream:
					return o.processStreaming(ctx, transcript, messages.turns, buffer)

				// TODO: Implement this
				// case LLMWithGeneralPrompt:
				case LLMWithPrompt:
					return o.processPromptOld(ctx, transcript, messages.turns, buffer)
				default:
					// Impossible state
					return nil, fmt.Errorf("unknown LLM type")
				}
			},
			TextToSpeechClient: o.textToSpeechClient,
		}
		callbacks := activeTurnCallbacks{ // TODO: See if these can be moved somewhere else and generalized, this could probably be moved to the top of the function
			OnResponseText: func(response string) {
				if o.orchestrateOptions.onResponse != nil {
					o.orchestrateOptions.onResponse(response)
				}
			},
			OnResponseTextEnd: func() {
				if o.orchestrateOptions.onResponseEnd != nil {
					o.orchestrateOptions.onResponseEnd()
				}
			},
			OnResponseSpeech: func(audio []byte) {
				if o.orchestrateOptions.onAudio != nil {
					o.orchestrateOptions.onAudio(audio)
				}
			},
			OnResponseSpeechEnd: func(transcript string) {
				if o.orchestrateOptions.onAudioEnded != nil {
					o.orchestrateOptions.onAudioEnded(transcript)
				}
			},
			OnFinalise: func(activeTurn *activeTurn) {
				span := trace.SpanFromContext(activeTurn.ctx)
				interruptionTypes := []string{}
				for _, interruption := range activeTurn.Interruptions {
					interruptionTypes = append(interruptionTypes, interruption.Type)
				}
				span.SetAttributes(attribute.StringSlice("assistant_turn.interruptions", interruptionTypes))
				span.SetAttributes(attribute.Int("assistant_turn.queued_triggers", len(o.transcripts)))
				span.End()
				// TODO: Check if turns IDs match
				if activeTurn := o.turns.activeTurn; activeTurn != nil {
					o.turns.turns = append(o.turns.turns, activeTurn.Turn)
					o.turns.activeTurn = nil
					o.promptEnded.Done()
				}
			},
		}
		if err := o.turns.processActiveTurn(ctx, components, callbacks,
			activeTurnConfig{IsSpeaking: o.IsSpeaking},
		); err != nil {
			err := fmt.Errorf("failed to process active turn: %v", err)
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			// TODO: Probably should be able to requeue the prompt or something
			// here
		}

	}
}

func (o *Orchestrator) processPromptOld(ctx context.Context, prompt string, messages []llms.Turn, buffer *textBuffer) (*llms.Turn, error) {
	if o.llm.(LLMWithPrompt) == nil {
		return nil, fmt.Errorf("LLM does not support prompting")
	}

	response, _ := o.llm.(LLMWithPrompt).Prompt(ctx, prompt,
		llms.WithTurns(messages...),
		llms.WithTools(o.tools...),
		llms.WithStream(buffer.AddChunk),
	)

	turns := llms.ToTurns(response)
	if len(turns) == 0 {
		log.Println("Warning: no turns returned for assistants turn")
		return nil, nil
	} else if len(turns) > 1 {
		log.Println("Warning: multiple turns returned for assistants turn")
	}
	return &turns[0], nil
}

func (o *Orchestrator) processStreaming(ctx context.Context, originalPrompt string, originalTurns []llms.Turn, buffer *textBuffer) (*llms.Turn, error) {
	span := trace.SpanFromContext(ctx)
	if o.llm.(LLMWithStream) == nil {
		return nil, fmt.Errorf("LLM does not support streaming")
	}
	llm := o.llm.(LLMWithStream)

	firstRun := true
	assistantTurn := llms.Turn{Role: llms.TurnRoleAssistant}
	for {
		var prompt *string
		turns := originalTurns
		if firstRun {
			prompt = &originalPrompt
			firstRun = false
		} else {
			turns = append(turns, assistantTurn)
		}

		stream := llm.PromptWithStream(ctx, prompt,
			llms.WithTurns(turns...),
			llms.WithTools(o.tools...),
		)

		var response strings.Builder
		toolCalls := []llms.ToolCall{}
		for chunk, err := range stream.Chunks(ctx) {
			if err != nil {
				// TODO: handle error
				span.RecordError(err)
				break
			}

			activeTurn := o.turns.activeTurn
			if activeTurn != nil && activeTurn.Cancelled {
				return nil, nil
			}

			switch chunk.(type) {
			// case llms.StreamRoleChunk:
			// case llms.StreamReasoningChunk:
			// case llms.StreamUsageChunk:
			// 	chunk := chunk.(llms.StreamUsageChunk)
			case llms.StreamContentChunk:
				chunk := chunk.(llms.StreamContentChunk)

				response.WriteString(chunk.Content())
				buffer.AddChunk(chunk.Content())

			case llms.StreamToolCallChunk:
				toolCalls = append(toolCalls, chunk.(llms.StreamToolCallChunk).ToolCall())
			}
		}

		for _, toolCall := range toolCalls {
			response, _ := o.callTool(ctx, toolCall)
			if response != nil {
				toolCall.Response = response.Content
			}
			assistantTurn.ToolCalls = append(assistantTurn.ToolCalls, toolCall)
		}

		if len(toolCalls) == 0 {
			assistantTurn.Content = response.String()
			return &assistantTurn, nil
		}
	}
}

func (o *Orchestrator) callTool(ctx context.Context, toolCall llms.ToolCall) (*llms.Turn, error) {
	toolName := toolCall.Name
	toolArguments := toolCall.Arguments
	if toolCall.Name == "" {
		toolName = toolCall.Function.Name
	}
	if toolCall.Arguments == "" {
		toolArguments = toolCall.Function.Arguments

	}
	ctx, span := tracer.Start(ctx, "execute tool")
	defer span.End()
	span.SetAttributes(attribute.String("tool.name", toolName))
	for _, tool := range o.tools {
		if tool.Function.Name == toolName {
			resp, err := tool.Execute(toolArguments)
			if err != nil {
				log.Println("Error executing tool:", err)
			}
			return &llms.Turn{
				ToolCallID: toolCall.ID,
				Role:       llms.TurnRoleAssistant,
				Content:    resp,
			}, nil
		}
	}

	return nil, fmt.Errorf("tool not found")
}
