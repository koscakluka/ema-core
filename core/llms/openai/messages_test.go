package openai

import (
	"testing"

	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/core/triggers"
)

func TestToOpenAIMessages_DoesNotTruncateHistoryAfterToolCalls(t *testing.T) {
	turns := []llms.TurnV1{
		{
			Trigger: triggers.NewUserPromptTrigger("first prompt"),
			ToolCalls: []llms.ToolCall{
				{
					ID:        "tool_1",
					Name:      "lookup_weather",
					Arguments: `{"city":"Prague"}`,
					Response:  `{"temp":21}`,
				},
			},
			Responses: []llms.TurnResponseV0{
				{
					Message:                 "It is 21C in Prague.",
					IsMessageFullyGenerated: true,
				},
			},
		},
		{
			Trigger: triggers.NewUserPromptTrigger("second prompt"),
			Responses: []llms.TurnResponseV0{
				{
					Message:                 "What else can I help with?",
					IsMessageFullyGenerated: true,
				},
			},
		},
	}

	messages := toOpenAIMessages("", turns)

	if len(messages) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(messages))
	}

	if messages[0].Type != messageTypeMessage || messages[0].Role != messageRoleUser || messages[0].Content != "first prompt" {
		t.Fatalf("unexpected first message: %+v", messages[0])
	}

	if messages[1].Type != messageTypeFunctionCall || messages[1].ToolCallID != "tool_1" {
		t.Fatalf("unexpected function call message: %+v", messages[1])
	}

	if messages[2].Type != messageTypeFunctionCallOutput || messages[2].ToolCallID != "tool_1" {
		t.Fatalf("unexpected function call output message: %+v", messages[2])
	}

	if messages[3].Type != messageTypeMessage || messages[3].Role != messageRoleAssistant || messages[3].Content != "It is 21C in Prague." {
		t.Fatalf("unexpected assistant message after tool call: %+v", messages[3])
	}

	if messages[4].Type != messageTypeMessage || messages[4].Role != messageRoleUser || messages[4].Content != "second prompt" {
		t.Fatalf("history truncated before second turn: %+v", messages[4])
	}

	if messages[5].Type != messageTypeMessage || messages[5].Role != messageRoleAssistant || messages[5].Content != "What else can I help with?" {
		t.Fatalf("unexpected final assistant message: %+v", messages[5])
	}
}
