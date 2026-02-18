package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"log"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var ErrLLMNotConfigured = errors.New("llm is not configured")

type llm struct {
	// client is the configured LLM implementation (streaming or prompt-based).
	client LLM
	// tools stores the effective tool list exposed to model calls.
	tools []llms.Tool
}

func newLLM() llm {
	return llm{}
}

func (runtime *llm) set(client LLM) {
	if runtime == nil {
		return
	}

	runtime.client = client
}

func (runtime *llm) setTools(tools ...llms.Tool) {
	if runtime == nil {
		return
	}

	runtime.tools = append([]llms.Tool(nil), tools...)
}

func (runtime *llm) appendTools(tools ...llms.Tool) {
	if runtime == nil || len(tools) == 0 {
		return
	}

	runtime.tools = append(runtime.tools, tools...)
}

func (runtime *llm) availableTools() []llms.Tool {
	if runtime == nil {
		return nil
	}

	tools := make([]llms.Tool, len(runtime.tools))
	copy(tools, runtime.tools)
	return tools
}

func (runtime *llm) generate(
	ctx context.Context,
	event llms.EventV0,
	conversation []llms.TurnV1,
	buffer *textBuffer,
	activeTurnCancelled func() bool,
) (*llms.Response, error) {
	if runtime == nil || runtime.client == nil {
		return nil, ErrLLMNotConfigured
	}

	switch client := runtime.client.(type) {
	case LLMWithStream:
		return runtime.processStreaming(ctx, client, event, conversation, buffer, activeTurnCancelled)

	case LLMWithPrompt:
		return runtime.processPrompt(ctx, client, event, conversation, buffer)

	default:
		return nil, fmt.Errorf("unknown LLM type")
	}
}

func (runtime *llm) processPrompt(ctx context.Context,
	client LLMWithPrompt,
	event llms.EventV0,
	conversations []llms.TurnV1,
	buffer *textBuffer,
) (*llms.Response, error) {
	response, err := client.Prompt(ctx, event.String(),
		llms.WithTurnsV1(conversations...),
		llms.WithTools(runtime.tools...),
		llms.WithStream(buffer.AddChunk),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to prompt llm: %w", err)
	}

	if len(response) == 0 {
		log.Println("Warning: no turns returned for assistants turn")
		return nil, nil
	} else if len(response) > 1 {
		log.Println("Warning: multiple turns returned for assistants turn")
	}
	return (*llms.Response)(&response[0]), nil
}

func (runtime *llm) processStreaming(ctx context.Context,
	client LLMWithStream,
	event llms.EventV0,
	conversation []llms.TurnV1,
	buffer *textBuffer,
	activeTurnCancelled func() bool,
) (*llms.Response, error) {
	span := trace.SpanFromContext(ctx)

	turn := llms.TurnV1{Event: event}
	for {
		stream := client.PromptWithStream(ctx, nil,
			llms.WithTurnsV1(append(conversation, turn)...),
			llms.WithTools(runtime.tools...),
		)

		var message strings.Builder
		toolCalls := []llms.ToolCall{}
		for chunk, err := range stream.Chunks(ctx) {
			if err != nil {
				err = fmt.Errorf("failed to stream llm response: %w", err)
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				return nil, err
			}

			if activeTurnCancelled != nil && activeTurnCancelled() {
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
			toolResponse, err := runtime.callTool(ctx, toolCall)
			if err != nil {
				err = fmt.Errorf("failed to call tool: %w", err)
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				return nil, err
			}
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
