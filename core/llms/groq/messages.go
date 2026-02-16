package groq

import (
	"github.com/koscakluka/ema-core/core/llms"
)

type message struct {
	Role       messageRole `json:"role"`
	Content    string      `json:"content"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	ToolCalls  []toolCall  `json:"tool_calls,omitempty"`
}

type messageRole string

const (
	messageRoleSystem    messageRole = "system"
	messageRoleUser      messageRole = "user"
	messageRoleAssistant messageRole = "assistant"
	messageRoleTool      messageRole = "tool"
)

type toolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function toolCallFunction `json:"function"`
}

type toolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func toMessages(instructions string, turns []llms.TurnV1) []message {
	messages := []message{}
	if instructions != "" {
		messages = append(messages, message{
			Role:    messageRoleSystem,
			Content: instructions,
		})
	}
	for _, turn := range turns {
		if turn.Event.String() != "" {
			messages = append(messages, message{
				Role:    messageRoleUser,
				Content: turn.Event.String(),
			})
		}

		if len(turn.ToolCalls) > 0 {
			msg := message{Role: messageRoleAssistant}
			responseMsgs := []message{}
			for _, tCall := range turn.ToolCalls {
				msg.ToolCalls = append(msg.ToolCalls, toolCall{
					ID:   tCall.ID,
					Type: "function",
					Function: toolCallFunction{
						Name:      tCall.Name,
						Arguments: tCall.Arguments,
					},
				})
				if tCall.Response != "" {
					responseMsgs = append(responseMsgs, message{
						Role:       messageRoleTool,
						Content:    tCall.Response,
						ToolCallID: tCall.ID,
					})
				}
			}

			messages = append(messages, msg)
			messages = append(messages, responseMsgs...)
		}
		for _, response := range turn.Responses {
			if !response.IsCompleted() {
				continue
			}
			msg := message{Role: messageRoleAssistant}
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
