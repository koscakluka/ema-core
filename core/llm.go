package orchestration

import (
	"context"
	"fmt"
	"strings"

	"log"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type llm struct {
	// client is the configured LLM implementation (streaming or prompt-based).
	client LLM
	// tools stores the effective tool list exposed to model calls.
	tools []llms.Tool

	// onResponse is called for each streamed response chunk.
	onResponse func(string)
	// onResponseEnd is called once response streaming is finished.
	onResponseEnd func()
}

func newLLM() llm {
	return llm{
		onResponse:    func(string) {},
		onResponseEnd: func() {},
	}
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

func (runtime *llm) setResponseCallbacks(onResponse func(string), onResponseEnd func()) {
	if runtime == nil {
		return
	}

	if onResponse != nil {
		runtime.onResponse = onResponse
	}
	if onResponseEnd != nil {
		runtime.onResponseEnd = onResponseEnd
	}
}

func (runtime *llm) availableTools() []llms.Tool {
	if runtime == nil {
		return nil
	}

	tools := make([]llms.Tool, len(runtime.tools))
	copy(tools, runtime.tools)
	return tools
}

func (runtime *llm) snapshot() llm {
	if runtime == nil {
		return llm{}
	}

	snapshot := llm{client: runtime.client}
	if len(runtime.tools) > 0 {
		snapshot.tools = make([]llms.Tool, len(runtime.tools))
		copy(snapshot.tools, runtime.tools)
	}
	snapshot.setResponseCallbacks(runtime.onResponse, runtime.onResponseEnd)

	return snapshot
}

func (runtime *llm) generate(
	ctx context.Context,
	trigger llms.TriggerV0,
	conversation []llms.TurnV1,
	onChunk func(string),
	activeTurnCancelled func() bool,
) (*llms.Response, error) {
	defer runtime.onResponseEnd()

	if runtime == nil || runtime.client == nil {
		return nil, nil
	}

	switch client := runtime.client.(type) {
	case LLMWithStream:
		return runtime.processStreaming(ctx, client, trigger, conversation, onChunk, activeTurnCancelled)

	case LLMWithPrompt:
		return runtime.processPrompt(ctx, client, trigger, conversation, onChunk)

	default:
		return nil, fmt.Errorf("unknown LLM type")
	}
}

func (runtime *llm) processPrompt(ctx context.Context,
	client LLMWithPrompt,
	trigger llms.TriggerV0,
	conversations []llms.TurnV1,
	onChunk func(string),
) (*llms.Response, error) {
	response, err := client.Prompt(ctx, trigger.String(),
		llms.WithTurnsV1(conversations...),
		llms.WithTools(runtime.tools...),
		llms.WithStream(func(chunk string) {
			if onChunk != nil {
				onChunk(chunk)
			}
			runtime.onResponse(chunk)
		}),
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
	trigger llms.TriggerV0,
	conversation []llms.TurnV1,
	onChunk func(string),
	activeTurnCancelled func() bool,
) (*llms.Response, error) {
	span := trace.SpanFromContext(ctx)

	turn := llms.TurnV1{Trigger: trigger}
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
				if onChunk != nil {
					onChunk(chunk.Content())
				}
				runtime.onResponse(chunk.Content())

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
