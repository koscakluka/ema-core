package openai

import "github.com/koscakluka/ema-core/core/llms"

type openAIMessage struct {
	Type messageType `json:"type"`

	Role    messageRole `json:"role,omitempty"`
	Content string      `json:"content,omitempty"`

	ToolCallID        string `json:"call_id,omitempty"`
	ToolCallName      string `json:"name,omitempty"`
	ToolCallArguments string `json:"arguments,omitempty"`
	ToolCallOutput    string `json:"output,omitempty"`
	ToolCallStatus    string `json:"status,omitempty"`
}

type messageRole string

const (
	messageRoleSystem    messageRole = "system"
	messageRoleDeveloper messageRole = "developer"
	messageRoleUser      messageRole = "user"
	messageRoleAssistant messageRole = "assistant"
	messageRoleTool      messageRole = "tool"
)

type messageType string

const (
	messageTypeMessage            messageType = "message"
	messageTypeFunctionCall       messageType = "function_call"
	messageTypeFunctionCallOutput messageType = "function_call_output"
)

func toOpenAIMessages(instructions string, turns []llms.TurnV1) []openAIMessage {
	messages := []openAIMessage{}
	if instructions != "" {
		messages = append(messages, openAIMessage{
			Role:    messageRoleDeveloper,
			Type:    messageTypeMessage,
			Content: instructions,
		})
	}

	for _, turn := range turns {
		messages = append(messages, openAIMessage{
			Type:    messageTypeMessage,
			Role:    messageRoleUser,
			Content: turn.Trigger.String(),
		})

		if len(turn.ToolCalls) > 0 {
			for _, toolCall := range turn.ToolCalls {
				messages = append(messages, openAIMessage{
					Type:              messageTypeFunctionCall,
					ToolCallID:        toolCall.ID,
					ToolCallName:      toolCall.Name,
					ToolCallArguments: toolCall.Arguments,
					ToolCallStatus:    "completed",
				})
				if toolCall.Response != "" {
					messages = append(messages, openAIMessage{
						Type:           messageTypeFunctionCallOutput,
						ToolCallID:     toolCall.ID,
						ToolCallOutput: toolCall.Response,
					})
				}
			}
		}
		for _, response := range turn.Responses {
			if !response.IsCompleted() {
				continue
			}
			msg := openAIMessage{
				Type: messageTypeMessage,
				Role: messageRoleAssistant,
			}
			if response.IsTyped {
				msg.Content = response.TypedMessage
			} else if response.IsSpoken {
				msg.Content = response.SpokenResponse
			} else {
				msg.Content = response.Message
			}

			messages = append(messages, msg)
		}
	}
	return messages
}
