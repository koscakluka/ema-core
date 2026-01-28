package orchestration

import (
	"context"
	"fmt"
	"strings"
	"time"

	"log"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (o *Orchestrator) startAssistantLoop() {
	for promptQueueItem := range o.transcripts {
		if o.turns.activeTurn != nil {
			o.promptEnded.Wait()
		}
		o.promptEnded.Add(1)

		ctx, mainSpan := tracer.Start(o.baseContext, "process turn")
		mainSpan.AddEvent("taken out of queue", trace.WithAttributes(attribute.Float64("assistant_turn.queued_time", time.Since(promptQueueItem.queuedAt).Seconds())))
		mainSpan.SetAttributes(attribute.Float64("assistant_turn.queued_time", time.Since(promptQueueItem.queuedAt).Seconds()))
		transcript := promptQueueItem.content

		messages := o.turns
		o.turns.Push(llms.Turn{
			Role:    llms.TurnRoleUser,
			Content: transcript,
		})

		if err := o.turns.setActiveTurn(ctx, llms.Turn{Role: llms.TurnRoleAssistant}, o.audioOutput); err != nil {
			// TODO: Probably should be able to requeue the prompt or something
			// here
			mainSpan.RecordError(fmt.Errorf("failed to set active turn: %v", err))
			continue
		}

		go o.passTextToTTS(ctx)
		go o.passSpeechToAudioOutput(ctx)

		ctx, span := tracer.Start(ctx, "generate response")
		var response *llms.Turn
		switch o.llm.(type) {
		case LLMWithStream:
			response, _ = o.processStreaming(ctx, transcript, messages.turns, &o.turns.activeTurn.textBuffer)

		// TODO: Implement this
		// case LLMWithGeneralPrompt:
		case LLMWithPrompt:
			response, _ = o.processPromptOld(ctx, transcript, messages.turns, &o.turns.activeTurn.textBuffer)
		default:
			// Impossible state
		}
		if response != nil {
			var toolCalls []string
			for _, toolCall := range response.ToolCalls {
				toolCalls = append(toolCalls, toolCall.Name)
			}
			span.SetAttributes(attribute.StringSlice("assistant_turn.tool_calls", toolCalls))
		}

		if activeTurn := o.turns.activeTurn; activeTurn != nil {
			activeTurn.textBuffer.ChunksDone()
			activeTurn.audioBuffer.ChunksDone()
		}
		activeTurn := o.turns.activeTurn
		if activeTurn != nil && response != nil && !activeTurn.Cancelled {
			activeTurn.Content = response.Content
			activeTurn.ToolCalls = response.ToolCalls
		} else {
			// TODO: Figure out how to handle this case
		}

		// NOTE: This is where the span ending is set, if there is a continue
		// above the span also needs to be ended there
		// TODO: Make sure that this is not a liability
		span.End()
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
	ctx, span := tracer.Start(ctx, "process streaming")
	defer span.End()
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
