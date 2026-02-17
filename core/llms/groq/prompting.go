package groq

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/jinzhu/copier"
	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/internal/utils"
)

const (
	url = "https://api.groq.com/openai/v1/chat/completions"

	endMessage  = "[DONE]"
	chunkPrefix = "data:"
)

func Prompt(
	_ context.Context,
	apiKey string,
	model string,
	prompt string,
	systemPrompt string,
	baseTools []llms.Tool,
	opts ...llms.PromptOption,
) ([]llms.Message, error) {
	// TODO: Split this and other prompting methods into raw and typed
	// variants
	// Name this method to Respond, GenerateResponse or RespondTo + version

	options := llms.PromptOptions{
		Tools:        slices.Clone(baseTools),
		Instructions: systemPrompt,
	}
	for _, opt := range opts {
		opt(&options)
	}

	messages := toMessages(options.Instructions, options.TurnsV1)
	messages = append(messages, message{
		Role:    messageRoleUser,
		Content: prompt,
	})

	var toolChoice *string
	var tools []Tool
	if options.Tools != nil {
		toolChoice = utils.Ptr("auto")

		if options.ForcedToolsCall {
			toolChoice = utils.Ptr("required")
		}
		copier.Copy(&tools, options.Tools)
	}

	responses := []llms.Message{}

	for {
		reqBody := requestBody{
			Model:      model,
			Messages:   messages,
			Stream:     true,
			Tools:      tools,
			ToolChoice: toolChoice,
		}

		requestBodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("error marshalling JSON: %w", err)
		}

		req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBodyBytes))
		if err != nil {
			return nil, fmt.Errorf("error creating HTTP request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("error sending request: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			// TODO: Retry depending on status, send back a message to the user
			// to indicate that something is going on
			log.Println("Non-OK HTTP status:", resp.Status)
		}

		toolCalls := []toolCall{}
		var response strings.Builder
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			chunk := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), chunkPrefix))

			if len(chunk) == 0 {
				continue
			}

			if chunk == endMessage {
				break
			}

			// log.Println("Chunk:", chunk)
			var responseBody streamingResponseBody
			err := json.Unmarshal([]byte(chunk), &responseBody)
			if err != nil {
				log.Println("Error unmarshalling JSON:", err)
				continue
			}
			if len(responseBody.Choices) == 0 {
				continue
			}
			if len(responseBody.Choices[0].Delta.ToolCalls) > 0 {
				toolCalls = append(toolCalls, responseBody.Choices[0].Delta.ToolCalls...)
			}

			content := responseBody.Choices[0].Delta.Content
			response.WriteString(content)
			if options.Stream != nil {
				options.Stream(content)
			}
		}

		if err := scanner.Err(); err != nil {
			log.Println("Error reading streamed response:", err)
		}
		if err := resp.Body.Close(); err != nil {
			log.Println("Error closing response body:", err)
		}

		messages = append(messages, message{
			Role:      messageRoleAssistant,
			Content:   response.String(),
			ToolCalls: toolCalls,
		})
		msg := llms.Message{
			Role:    llms.MessageRoleAssistant,
			Content: response.String(),
		}
		for _, toolCall := range toolCalls {
			msg.ToolCalls = append(msg.ToolCalls, llms.ToolCall{
				ID:        toolCall.ID,
				Type:      toolCall.Type,
				Name:      toolCall.Function.Name,
				Arguments: toolCall.Function.Arguments,
				Function:  llms.ToolCallFunction{Name: toolCall.Function.Name, Arguments: toolCall.Function.Arguments},
			})
		}
		responses = append(responses, msg)
		if len(toolCalls) == 0 {
			llmResponses := []llms.Message{}
			copier.Copy(&llmResponses, responses)
			return llmResponses, nil
		}

		for _, toolCall := range toolCalls {
			for _, tool := range options.Tools {
				if tool.Function.Name == toolCall.Function.Name {
					resp, err := tool.Execute(toolCall.Function.Arguments)
					if err != nil {
						log.Println("Error executing tool:", err)
					}
					messages = append(messages, message{
						ToolCallID: toolCall.ID,
						Role:       messageRoleTool,
						Content:    resp,
					})
					responses = append(responses, llms.Message{
						ToolCallID: toolCall.ID,
						Role:       llms.MessageRoleTool,
						Content:    resp,
					})
				}
			}

		}

	}
}

type requestBody struct {
	Model      string    `json:"model"`
	Messages   []message `json:"messages"`
	Stream     bool      `json:"stream"`
	ToolChoice *string   `json:"tool_choice,omitempty"`
	Tools      []Tool    `json:"tools,omitempty"`
}

type streamingResponseBody struct {
	Choices []struct {
		Delta struct {
			Role         string     `json:"role,omitempty"`
			Content      string     `json:"content,omitempty"`
			ToolCalls    []toolCall `json:"tool_calls,omitempty"`
			Reasoning    string     `json:"reasoning,omitempty"`
			Channel      string     `json:"channel,omitempty"`
			FinishReason *string    `json:"finish_reason,omitempty"`
		} `json:"delta"`
	} `json:"choices"`
	Usage *struct {
		QueueTime               float64 `json:"queue_time"`
		PromptTokens            int     `json:"prompt_tokens"`
		PromptTime              float64 `json:"prompt_time"`
		CompletionTokens        int     `json:"completion_tokens"`
		CompletionTime          float64 `json:"completion_time"`
		TotalTokens             int     `json:"total_tokens"`
		TotalTime               float64 `json:"total_time"`
		CompletionTokensDetails *struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		}
	} `json:"usage"`
}
