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

type eventQueueItem struct {
	event    llms.EventV0
	queuedAt time.Time
}

func (o *Orchestrator) queueEvent(event llms.EventV0) {
	o.eventQueue <- eventQueueItem{event: event, queuedAt: time.Now()}
}

func (o *Orchestrator) startAssistantLoop() {
	for promptQueueItem := range o.eventQueue {
		// NOTE: This loop is a single queue consumer, so turns are already
		// processed sequentially without extra synchronization.
		func(promptQueueItem eventQueueItem) {
			ctx, span := tracer.Start(o.baseContext, "process turn")
			defer span.End()

			queuedTime := time.Since(promptQueueItem.queuedAt).Seconds()
			span.AddEvent("taken out of queue", trace.WithAttributes(attribute.Float64("assistant_turn.queued_time", queuedTime)))
			span.SetAttributes(attribute.Float64("assistant_turn.queued_time", queuedTime))

			event := promptQueueItem.event

			components := activeTurnComponents{
				AudioOutput: o.audioOutput,
				ResponseGenerator: func() (func(context.Context, llms.EventV0, []llms.TurnV1, *textBuffer) (*llms.Response, error), error) {
					switch o.llm.(type) {
					case LLMWithStream:
						return o.processStreaming, nil

					// TODO: Implement this
					// case LLMWithGeneralPrompt:
					case LLMWithPrompt:
						return o.processPromptOld, nil
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
				OnFinalise: func(finalisedTurn *activeTurn) {
					interruptionTypes := []string{}
					for _, interruption := range finalisedTurn.Interruptions {
						interruptionTypes = append(interruptionTypes, interruption.Type)
					}
					span.SetAttributes(attribute.StringSlice("assistant_turn.interruptions", interruptionTypes))
					span.SetAttributes(attribute.Int("assistant_turn.queued_events", len(o.eventQueue)))

					if activeTurn := o.conversation.activeTurn; activeTurn != nil {
						if activeTurn.TurnV1.ID != finalisedTurn.TurnV1.ID {
							err := fmt.Errorf("turn IDs do not match")
							span.RecordError(err)
							span.SetStatus(codes.Error, err.Error())
							o.conversation.turns = append(o.conversation.turns, finalisedTurn.TurnV1)
							return
						}

						o.conversation.turns = append(o.conversation.turns, finalisedTurn.TurnV1)
						o.conversation.activeTurn = nil
						return
					}

					err := fmt.Errorf("active turn missing during finalisation")
					span.RecordError(err)
					span.SetStatus(codes.Error, err.Error())
					o.conversation.turns = append(o.conversation.turns, finalisedTurn.TurnV1)
				},
			}
			if err := o.conversation.processActiveTurn(ctx, event, components, callbacks,
				activeTurnConfig{IsSpeaking: o.IsSpeaking},
			); err != nil {
				err := fmt.Errorf("failed to process active turn: %v", err)
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				// TODO: Probably should be able to requeue the prompt or something
				// here
			}
		}(promptQueueItem)
	}
}

func (o *Orchestrator) processPromptOld(ctx context.Context, event llms.EventV0, conversations []llms.TurnV1, buffer *textBuffer) (*llms.Response, error) {
	if o.llm.(LLMWithPrompt) == nil {
		return nil, fmt.Errorf("LLM does not support prompting")
	}

	response, _ := o.llm.(LLMWithPrompt).Prompt(ctx, event.String(),
		llms.WithTurnsV1(conversations...),
		llms.WithTools(o.tools...),
		llms.WithStream(buffer.AddChunk),
	)

	if len(response) == 0 {
		log.Println("Warning: no turns returned for assistants turn")
		return nil, nil
	} else if len(response) > 1 {
		log.Println("Warning: multiple turns returned for assistants turn")
	}
	return (*llms.Response)(&response[0]), nil
}

func (o *Orchestrator) processStreaming(ctx context.Context, event llms.EventV0, conversation []llms.TurnV1, buffer *textBuffer) (*llms.Response, error) {
	span := trace.SpanFromContext(ctx)
	if o.llm.(LLMWithStream) == nil {
		return nil, fmt.Errorf("LLM does not support streaming")
	}
	llm := o.llm.(LLMWithStream)

	turn := llms.TurnV1{Event: event}
	for {
		stream := llm.PromptWithStream(ctx, nil,
			llms.WithTurnsV1(append(conversation, turn)...),
			llms.WithTools(o.tools...),
		)

		var message strings.Builder
		toolCalls := []llms.ToolCall{}
		for chunk, err := range stream.Chunks(ctx) {
			if err != nil {
				// TODO: handle error
				span.RecordError(err)
				break
			}

			activeTurn := o.conversation.activeTurn
			if activeTurn != nil && activeTurn.IsCancelled() {
				// TODO: This is probably not the best way to handle this,
				// returning something would make more sense
				return nil, nil
			}

			switch chunk.(type) {
			// case llms.StreamRoleChunk:
			// case llms.StreamReasoningChunk:
			// case llms.StreamUsageChunk:
			// 	chunk := chunk.(llms.StreamUsageChunk)
			case llms.StreamContentChunk:
				chunk := chunk.(llms.StreamContentChunk)

				message.WriteString(chunk.Content())
				buffer.AddChunk(chunk.Content())

			case llms.StreamToolCallChunk:
				toolCalls = append(toolCalls, chunk.(llms.StreamToolCallChunk).ToolCall())
			}
		}

		for _, toolCall := range toolCalls {
			toolResponse, _ := o.callTool(ctx, toolCall)
			if toolResponse != nil {
				toolCall.Response = toolResponse.Response
			}
			turn.ToolCalls = append(turn.ToolCalls, toolCall)
		}

		if len(toolCalls) == 0 {
			return &llms.Response{
				Content:   message.String(),
				ToolCalls: turn.ToolCalls,
			}, nil
		}
	}
}

func (o *Orchestrator) callTool(ctx context.Context, toolCall llms.ToolCall) (*llms.ToolCall, error) {
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
			return &llms.ToolCall{
				ID:       toolCall.ID,
				Response: resp,
			}, nil
		}
	}

	return nil, fmt.Errorf("tool not found")
}
