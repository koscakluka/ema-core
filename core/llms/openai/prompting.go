package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/internal/utils"
)

func Prompt(
	_ context.Context,
	apiKey string,
	model string,
	prompt string,
	systemPrompt string,
	opts ...llms.GeneralPromptOption,
) (*llms.Message, error) {
	options := llms.GeneralPromptOptions{BaseOptions: llms.BaseOptions{Instructions: systemPrompt}}
	for _, opt := range opts {
		opt.ApplyToGeneral(&options)
	}

	messages := toOpenAIMessages(options.BaseOptions.Instructions, options.BaseOptions.TurnsV1)
	messages = append(messages, openAIMessage{
		Type:    messageTypeMessage,
		Role:    messageRoleUser,
		Content: prompt,
	})

	var toolChoice *string
	var tools []openAITool
	if options.Tools != nil {
		toolChoice = utils.Ptr("auto")

		if options.ForcedToolsCall {
			toolChoice = utils.Ptr("required")
		}

		tools = toOpenAITools(options.Tools)
	}

	reqBody := requestBody{
		Model:      model,
		Input:      messages,
		Stream:     false,
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
	// TODO: Add org and project headers

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// TODO: Retry depending on status, send back a message to the user
		// to indicate that something is going on
		return nil, fmt.Errorf("non-OK HTTP status: %s", resp.Status)
		// TODO: OpenAI provides a body with the error message
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}
	defer resp.Body.Close()

	var responseBody generalResponseBody
	if err := json.Unmarshal(bodyBytes, &responseBody); err != nil {
		return nil, fmt.Errorf("error unmarshalling response body: %w", err)
	}

	response := llms.Message{}

	for _, output := range responseBody.Output {
		var outputType generalResponseBodyOutputType
		if err := json.Unmarshal(output, &outputType); err != nil {
			return nil, fmt.Errorf("error unmarshalling output type: %w", err)
		}

		switch outputType.Type {
		case generalResponseBodyOutputTypeMessage:
			var outputMessage generalResponseBodyOutputMessage
			if err := json.Unmarshal(output, &outputMessage); err != nil {
				return nil, fmt.Errorf("error unmarshalling output message: %w", err)
			}
			response.Role = llms.MessageRoleAssistant
			for _, content := range outputMessage.Content {
				var contentType generalResponseBodyOutputMessageType
				if err := json.Unmarshal(content, &contentType); err != nil {
					return nil, fmt.Errorf("error unmarshalling output message content: %w", err)
				}
				switch contentType.Type {
				case "output_text":
					var outputText generalResponseBodyOutputMessageContentOutputText
					if err := json.Unmarshal(content, &outputText); err != nil {
						return nil, fmt.Errorf("error unmarshalling output message content output text: %w", err)
					}
					response.Content = outputText.Text
				case "refusal":
					var outputRefusal generalResponseBodyOutputMessageContentRefusal
					if err := json.Unmarshal(content, &outputRefusal); err != nil {
						return nil, fmt.Errorf("error unmarshalling output message content refusal: %w", err)
					}
					response.Content = outputRefusal.Refusal
				}
			}
			// response.Content = outputMessage.Content

		case generalResponseBodyOutputTypeFunctionCall:
			var outputFunctionCall generalResponseBodyOutputFunctionCall
			if err := json.Unmarshal(output, &outputFunctionCall); err != nil {
				return nil, fmt.Errorf("error unmarshalling output function call: %w", err)
			}
			response.ToolCalls = append(response.ToolCalls, llms.ToolCall{
				ID:        outputFunctionCall.CallID,
				Type:      "function",
				Name:      outputFunctionCall.Name,
				Arguments: outputFunctionCall.Arguments,
				Function:  llms.ToolCallFunction{Name: outputFunctionCall.Name, Arguments: outputFunctionCall.Arguments},
			})

		case generalResponseBodyOutputTypeReasoning:
			// TODO: Handle reasoning
		}
	}

	return &response, nil
}

type requestBody struct {
	Model      string                `json:"model"`
	Input      []openAIMessage       `json:"input"`
	Stream     bool                  `json:"stream"`
	ToolChoice *string               `json:"tool_choice,omitempty"`
	Tools      []openAITool          `json:"tools,omitempty"`
	Reasoning  *requestBodyReasoning `json:"reasoning,omitempty"`
}

type requestBodyReasoning struct {
	Effort  *string `json:"effort,omitempty"`
	Summary *string `json:"summary,omitempty"`
}

type generalResponseBody struct {
	Output []json.RawMessage `json:"output"`
	// TODO: Find a way to pass usage in the response
	// Usage  responseBodyUsage `json:"usage"`
}

type generalResponseBodyOutputType struct {
	// Type is the type of the output item.
	Type generalResponseBodyOutputTypeType `json:"type"`
}

type generalResponseBodyOutputBase struct {
	// ID is the unique ID of the output item.
	ID string `json:"id"`
	// Type is the type of the output item.
	// Type generalResponseBodyOutputTypeType `json:"type"`
	// Status is the status of the message input. One of 'in_progress', 'completed', or 'incomplete'.
	// Status string `json:"status"`
}

type generalResponseBodyOutputMessage struct {
	generalResponseBodyOutputBase
	// Content is the content of the output message.
	Content []json.RawMessage `json:"content,omitempty"`
	// Role is the role of the output message. Always 'assistant'
	// Role *string `json:"role,omitempty"`
}

type generalResponseBodyOutputMessageType struct {
	// Type is the type of the output message. 'output_text' or 'refusal'.
	Type string `json:"type"`
}

// generalResponseBodyOutputMessageContentOutputText is text output from the
// model.
type generalResponseBodyOutputMessageContentOutputText struct {
	// TODO:
	// annotations array
	// The annotations of the text output.
	//
	// TODO:
	// logprobs array

	// Text is the text output from the model.
	Text string `json:"text"`
}

// generalResponseBodyOutputMessageContentRefusal is a refusal from the model.
type generalResponseBodyOutputMessageContentRefusal struct {
	// Refusal is the refusal explanation from the model.
	Refusal string `json:"refusal"`
}

type generalResponseBodyOutputFunctionCall struct {
	generalResponseBodyOutputBase
	// CallID is the unique ID of the function tool call generated by the model.
	CallID string `json:"call_id"`
	// Name is the name of the function to run.
	Name string `json:"name"`
	// Arguments is the arguments string to pass to the function.
	Arguments string `json:"arguments"`
}

// generalResponseBodyOutputReasoning is a description of the chain of thought
// used by a reasoning model while generating a response. Be sure to include
// these items in your input to the Responses API for subsequent turns of a
// conversation if you are manually managing context.
type generalResponseBodyOutputReasoning struct {
	generalResponseBodyOutputBase
	// Summary is the reasoning summary content.
	Summary []struct {
		Text string `json:"text"`
		//Type string `json:"type"`
	} `json:"summary"`

	// Content is the reasoning text content.
	Content []struct {
		Text string `json:"text"`
		// Type string `json:"type"`
	} `json:"content"`
	// EncryptedContent is the encrypted content of the reasoning item -
	// populated when a response is generated with reasoning.encrypted_content
	// in the include parameter.
	EncryptedContent *string `json:"encrypted_content,omitempty"`
}

type generalResponseBodyOutputTypeType string

const (
	generalResponseBodyOutputTypeMessage      generalResponseBodyOutputTypeType = "message"
	generalResponseBodyOutputTypeFunctionCall generalResponseBodyOutputTypeType = "function_call"
	generalResponseBodyOutputTypeReasoning    generalResponseBodyOutputTypeType = "reasoning"
)

// responseBodyUsage represents token usage details including input tokens,
// output tokens, a breakdown of output tokens, and the total tokens used.
type responseBodyUsage struct {
	// InputTokens represents the number of input tokens.
	InputTokens int `json:"input_tokens"`
	// InputTokensDetails represents a detailed breakdown of the input tokens.
	InputTokensDetails *struct {
		// CachedTokens represents the number of tokens that were retrieved from the
		// cache. [More on prompt caching](https://platform.openai.com/docs/guides/prompt-caching)
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	// OutputTokens represents the number of output tokens.
	OutputTokens int `json:"output_tokens"`
	// OutputTokensDetails represents a detailed breakdown of the output tokens.
	OutputTokensDetails *struct {
		// ReasoningTokens represents the number of reasoning tokens.
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
	// TotalTokens represents the total number of tokens used.
	TotalTokens int `json:"total_tokens"`
}
