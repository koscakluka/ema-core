package orchestration

import (
	"context"
	"fmt"

	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func orchestrationTools(o *Orchestrator) []llms.Tool {
	return []llms.Tool{
		llms.NewTool("recording_control", "Turn on or off sound recording, might be referred to as 'listening'",
			map[string]llms.ParameterBase{
				"is_recording": {Type: "boolean", Description: "Whether to record or not"},
			},
			func(parameters struct {
				IsRecording bool `json:"is_recording"`
			}) (string, error) {
				o.SetAlwaysRecording(parameters.IsRecording)
				return "Success. Respond with a very short phrase", nil
			}),
		llms.NewTool("speaking_control", "Turn off agent's speaking ability. Might be referred to as 'muting'",
			map[string]llms.ParameterBase{
				"is_speaking": {Type: "boolean", Description: "Wheather to speak or not"},
			},
			func(parameters struct {
				IsSpeaking bool `json:"is_speaking"`
			}) (string, error) {
				o.SetSpeaking(parameters.IsSpeaking)
				return "Success. Respond with a very short phrase", nil
			}),
	}
}

func (o *Orchestrator) callTool(ctx context.Context, toolCall llms.ToolCall) (*llms.ToolCall, error) {
	runtimeLLM := o.llm.snapshot()
	return runtimeLLM.callTool(ctx, toolCall)
}

func (runtime *llm) callTool(ctx context.Context, toolCall llms.ToolCall) (*llms.ToolCall, error) {
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
	for _, tool := range runtime.tools {
		if tool.Function.Name == toolName {
			resp, err := tool.Execute(toolArguments)
			if err != nil {
				err = fmt.Errorf("failed to execute tool %q: %w", toolName, err)
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				return nil, err
			}
			return &llms.ToolCall{
				ID:       toolCall.ID,
				Response: resp,
			}, nil
		}
	}

	err := fmt.Errorf("tool not found: %s", toolName)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	return nil, err
}
