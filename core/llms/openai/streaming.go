package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/internal/utils"
)

const (
	url = "https://api.openai.com/v1/responses"

	eventPrefix = "event:"
	chunkPrefix = "data:"
)

func PromptWithStream(
	_ context.Context,
	apiKey string,
	model string,
	prompt *string,
	systemPrompt string,
	opts ...llms.StreamingPromptOption,
) *Stream {
	options := llms.StreamingPromptOptions{
		GeneralPromptOptions: llms.GeneralPromptOptions{
			BaseOptions: llms.BaseOptions{Instructions: systemPrompt},
		},
	}
	for _, opt := range opts {
		opt.ApplyToStreaming(&options)
	}

	messages := toOpenAIMessages(options.BaseOptions.Instructions, options.BaseOptions.TurnsV1)
	if prompt != nil {
		messages = append(messages, openAIMessage{
			Type:    messageTypeMessage,
			Role:    messageRoleUser,
			Content: *prompt,
		})
	}

	var tools []openAITool
	if options.GeneralPromptOptions.Tools != nil {
		tools = toOpenAITools(options.GeneralPromptOptions.Tools)
	}

	return &Stream{
		apiKey:   apiKey,
		model:    model,
		tools:    tools,
		messages: messages,
	}

}

type Stream struct {
	apiKey string

	model    string
	tools    []openAITool
	messages []openAIMessage
}

func (s *Stream) Chunks(ctx context.Context) func(func(llms.StreamChunk, error) bool) {
	return func(yield func(llms.StreamChunk, error) bool) {
		var toolChoice *string
		if s.tools != nil {
			toolChoice = utils.Ptr("auto")
		}

		reqBody := requestBody{
			Model:      s.model,
			Input:      s.messages,
			Stream:     true,
			Tools:      s.tools,
			ToolChoice: toolChoice,
			// TODO: Make sure reasoning can be tweaked and activated
			// OpenAI requires the organisation to be approved before this can be
			// used. Probably some way of caching the result of the response would
			// be useful and skiping reasoning in those cases instead of failing.
			// Reasoning: &requestBodyReasoning{
			// 	Effort:  utils.Ptr("low"),
			// 	Summary: utils.Ptr("auto"),
			// },
		}

		requestBodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			yield(nil, fmt.Errorf("error marshalling JSON: %w", err))
			return
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBodyBytes))
		if err != nil {
			yield(nil, fmt.Errorf("error creating HTTP request: %w", err))
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
		// TODO: Add org and project headers

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			yield(nil, fmt.Errorf("error sending request: %w", err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			// TODO: Retry depending on status, send back a message to the user
			// to indicate that something is going on
			yield(nil, fmt.Errorf("non-OK HTTP status: %s", resp.Status))
			return
		}

		usage := llms.Usage{}
		lapTime := time.Now()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			chunk := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), chunkPrefix))

			if len(chunk) == 0 {
				continue
			}

			if !strings.HasPrefix(chunk, "event:") {
				// HACK: We probably shouldn't, but let's see if this breaks
				// anything
				continue
			}

			event := strings.TrimSpace(strings.TrimPrefix(chunk, eventPrefix))
			// log.Println("Event:", event)

			scanner.Scan()
			chunk = strings.TrimSpace(strings.TrimPrefix(scanner.Text(), chunkPrefix))
			// log.Println("Chunk:", chunk)

			switch streamingEventType(event) {
			case streamingEventResponseCreated:
				lapTime = time.Now()

			case streamingEventResponseQueued:
				lapTime = time.Now()

			case streamingEventResponseInProgress:
				usage.QueueTime = time.Since(lapTime).Seconds()
				lapTime = time.Now()

			case streamingEventResponseOutputItemAdded:
				usage.InputProcessingTimes = time.Since(lapTime).Seconds()
				usage.PromptTime = time.Since(lapTime).Seconds()
				lapTime = time.Now()

			case streamingEventResponseOutputTextDelta:
				var responseBody streamingBodyResponseTextDelta
				if err := json.Unmarshal([]byte(chunk), &responseBody); err != nil {
					if !yield(nil, fmt.Errorf("error unmarshalling JSON: %w", err)) {
						return
					}
					continue
				}
				if !yield(StreamContentChunk{finishReason: nil, content: responseBody.Delta}, nil) {
					return
				}

			case streamingEventResponseOutputItemDone:
				var responseBody streamingBodyOutputItemDone[streamingBodyOutputItemDoneItem]
				if err := json.Unmarshal([]byte(chunk), &responseBody); err != nil {
					if !yield(nil, fmt.Errorf("error unmarshalling JSON: %w", err)) {
						return
					}
					continue
				}
				switch responseBody.Item.Type {
				case "function_call":
					var responseBody streamingBodyOutputItemDone[streamingBodyOutputItemDoneItemFunctionCall]
					if err := json.Unmarshal([]byte(chunk), &responseBody); err != nil {
						if !yield(nil, fmt.Errorf("error unmarshalling JSON: %w", err)) {
							return
						}
						continue
					}
					if !yield(StreamToolCallChunk{
						toolCall: llms.ToolCall{
							ID:        responseBody.Item.CallID,
							Type:      "function",
							Name:      responseBody.Item.Name,
							Arguments: responseBody.Item.Arguments,
							Function: llms.ToolCallFunction{
								Name:      responseBody.Item.Name,
								Arguments: responseBody.Item.Arguments,
							},
						},
					}, nil) {
						return
					}
				}

			case streamingEventResponseReasoningTextDelta,
				streamingEventResponseReasoningSummaryTextDelta:
				// TODO: Find out when streamingEventResponseReasoningTextDelta is
				// activated, so far, didn't get it.
				var responseBody streamingBodyResponseTextDelta
				err := json.Unmarshal([]byte(chunk), &responseBody)
				if err != nil {
					if !yield(nil, fmt.Errorf("error unmarshalling JSON: %w", err)) {
						return
					}
					continue
				}
				// TODO: Figure out what is the channel
				// channel:      delta.Channel,
				if !yield(StreamReasoningChunk{reasoning: responseBody.Delta}, nil) {
					return
				}

			case streamingEventResponseCompleted:
				usage.CompletionTime = time.Since(lapTime).Seconds()
				usage.OutputProcessingTime = time.Since(lapTime).Seconds()
				usage.TotalTime = usage.InputProcessingTimes + usage.OutputProcessingTime

				var responseBody streamingBodyResponseCompleted
				if err := json.Unmarshal([]byte(chunk), &responseBody); err != nil {
					if !yield(StreamUsageChunk{usage: usage}, nil) {
						return
					}
					if !yield(nil, fmt.Errorf("error unmarshalling JSON: %w", err)) {
						return
					}
					continue
				}

				if responseBody.Response.Usage != nil {
					usage.InputTokens = responseBody.Response.Usage.InputTokens
					usage.PromptTokens = responseBody.Response.Usage.InputTokens
					usage.OutputTokens = responseBody.Response.Usage.OutputTokens
					usage.CompletionTokens = responseBody.Response.Usage.OutputTokens
					usage.TotalTokens = responseBody.Response.Usage.TotalTokens

					if responseBody.Response.Usage.InputTokensDetails != nil {
						usage.InputTokensDetails = &llms.InputTokensDetails{
							CachedTokens: responseBody.Response.Usage.InputTokensDetails.CachedTokens,
						}
					}
					if responseBody.Response.Usage.OutputTokensDetails != nil {
						usage.OutputTokensDetails = &llms.OutputTokensDetails{
							ReasoningTokens: responseBody.Response.Usage.OutputTokensDetails.ReasoningTokens,
						}
						usage.CompletionTokensDetails = &llms.CompletionTokensDetails{
							ReasoningTokens: responseBody.Response.Usage.OutputTokensDetails.ReasoningTokens,
						}
					}
				}

				if !yield(StreamUsageChunk{usage: usage}, nil) {
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			yield(nil, fmt.Errorf("error reading streamed response: %w", err))
			return
		}
	}
}

type streamingEventType string

const (
	streamingEventResponseOutputTextDelta           streamingEventType = "response.output_text.delta"
	streamingEventResponseOutputItemAdded           streamingEventType = "response.output_item.added"
	streamingEventResponseOutputItemDone            streamingEventType = "response.output_item.done"
	streamingEventResponseReasoningTextDelta        streamingEventType = "response.reasoning_text.delta"
	streamingEventResponseReasoningSummaryTextDelta streamingEventType = "response.reasoning_summary_text.delta"
	streamingEventResponseCreated                   streamingEventType = "response.created"
	streamingEventResponseQueued                    streamingEventType = "response.queued"
	streamingEventResponseInProgress                streamingEventType = "response.in_progress"
	streamingEventResponseCompleted                 streamingEventType = "response.completed"
)

type streamingBodyResponseTextDelta struct {
	Delta string `json:"delta"`
}

type streamingBodyOutputItemDone[T any] struct {
	Item T `json:"item"`
}

type streamingBodyOutputItemDoneItem struct {
	Type string `json:"type"`
}

type streamingBodyOutputItemDoneItemFunctionCall struct {
	Arguments string `json:"arguments"`
	CallID    string `json:"call_id"`
	Name      string `json:"name"`
}

// streamingBodyResponseCompleted is emitted when the model response is complete
type streamingBodyResponseCompleted struct {
	Response struct {
		// Usage represents token usage details including input tokens, output
		// tokens, a breakdown of output tokens, and the total tokens used.
		Usage *responseBodyUsage `json:"usage"`
	} `json:"response"`
}

type StreamRoleChunk struct {
	finishReason *string
	role         string
}

func (s StreamRoleChunk) FinishReason() *string {
	return s.finishReason
}

func (s StreamRoleChunk) Role() string {
	return s.role
}

type StreamReasoningChunk struct {
	finishReason *string
	reasoning    string
	channel      string
}

func (s StreamReasoningChunk) FinishReason() *string {
	return s.finishReason
}

func (s StreamReasoningChunk) Reasoning() string {
	return s.reasoning
}

func (s StreamReasoningChunk) Channel() string {
	return s.channel
}

type StreamContentChunk struct {
	finishReason *string
	content      string
}

func (s StreamContentChunk) FinishReason() *string {
	return s.finishReason
}

func (s StreamContentChunk) Content() string {
	return s.content
}

type StreamToolCallChunk struct {
	finishReason *string
	toolCall     llms.ToolCall
}

func (s StreamToolCallChunk) FinishReason() *string {
	return s.finishReason
}

func (s StreamToolCallChunk) ToolCall() llms.ToolCall {
	return s.toolCall
}

type StreamUsageChunk struct {
	finishReason *string
	usage        llms.Usage
}

func (s StreamUsageChunk) FinishReason() *string {
	return s.finishReason
}

func (s StreamUsageChunk) Usage() llms.Usage {
	return s.usage
}
