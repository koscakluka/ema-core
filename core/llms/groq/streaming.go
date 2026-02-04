package groq

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jinzhu/copier"
	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/internal/utils"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func PromptWithStream(
	_ context.Context,
	apiKey string,
	model string,
	prompt *string,
	systemPrompt string,
	baseTools []llms.Tool,
	opts ...llms.StreamingPromptOption,
) *Stream {
	options := llms.StreamingPromptOptions{
		GeneralPromptOptions: llms.GeneralPromptOptions{
			BaseOptions: llms.BaseOptions{
				Instructions: systemPrompt,
			},
			Tools: slices.Clone(baseTools),
		},
	}
	for _, opt := range opts {
		opt.ApplyToStreaming(&options)
	}

	messages := toMessages(options.BaseOptions.Instructions, options.BaseOptions.TurnsV1)
	if prompt != nil {
		messages = append(messages, message{
			Role:    messageRoleUser,
			Content: *prompt,
		})
	}

	var tools []Tool
	if options.GeneralPromptOptions.Tools != nil {
		copier.Copy(&tools, options.GeneralPromptOptions.Tools)
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
	tools    []Tool
	messages []message
}

func (s *Stream) Chunks(ctx context.Context) func(func(llms.StreamChunk, error) bool) {
	requestToFirstTokenTime := time.Time{}
	setRequestToFirstTokenTime := func(span trace.Span) {
		if requestToFirstTokenTime.IsZero() {
			return
		}
		span.SetAttributes(attribute.Float64("response.request_to_first_token_time", time.Since(requestToFirstTokenTime).Seconds()))
		span.AddEvent("received first chunk")
		requestToFirstTokenTime = time.Time{}
	}

	// TODO: See if this needs the ctx passed, or if the context should be saved.
	// Intuitively it seems like it should be saved, but there is no reason to
	// assume that the chunking will be invoked in the same place where prompting will
	return func(yield func(llms.StreamChunk, error) bool) {
		ctx, span := tracer.Start(ctx, "prompt llm stream")
		defer span.End()
		span.SetAttributes(attribute.String("request.model", s.model))
		var toolNames []string
		for _, tool := range s.tools {
			toolNames = append(toolNames, tool.Function.Name)
		}
		span.SetAttributes(attribute.StringSlice("request.available_tools", toolNames))

		var toolChoice *string
		if s.tools != nil {
			toolChoice = utils.Ptr("auto")
		}

		reqBody := requestBody{
			Model:      s.model,
			Messages:   s.messages,
			Stream:     true,
			Tools:      s.tools,
			ToolChoice: toolChoice,
		}

		requestBodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			err = fmt.Errorf("error marshalling JSON: %w", err)
			span.RecordError(err)
			yield(nil, err)
			return
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBodyBytes))
		if err != nil {
			err = fmt.Errorf("error creating HTTP request: %w", err)
			span.RecordError(err)
			yield(nil, err)
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+s.apiKey)

		span.SetAttributes(attribute.String("request.url", req.URL.String()))
		client := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport,
			otelhttp.WithSpanNameFormatter(func(operationName string, request *http.Request) string {
				return operationName + " " + request.URL.Path
			}),
		)}
		requestToFirstTokenTime = time.Now()
		span.AddEvent("request started")
		resp, err := client.Do(req)
		if err != nil {
			err = fmt.Errorf("error sending request: %w", err)
			span.RecordError(err)
			yield(nil, err)
			return
		}
		defer resp.Body.Close()

		span.SetAttributes(attribute.Int("response.status_code", resp.StatusCode))
		if resp.StatusCode != http.StatusOK {
			if errorBody, err := io.ReadAll(resp.Body); err != nil {
				err = fmt.Errorf("error reading error body: %w", err)
				span.RecordError(err)
				span.SetAttributes(attribute.String("error", err.Error()))
			} else {
				span.SetAttributes(attribute.String("response.error", string(errorBody)))
			}

			// TODO: Retry depending on status, send back a message to the user
			// to indicate that something is going on
			err := fmt.Errorf("non-OK HTTP status: %s", resp.Status)
			span.RecordError(err)
			yield(nil, fmt.Errorf("non-OK HTTP status: %s", resp.Status))
			return
		}

		toolCalls := []toolCall{}
		defer func() {
			toolNames := []string{}
			for _, toolCall := range toolCalls {
				toolNames = append(toolNames, toolCall.Function.Name)
			}
			span.SetAttributes(attribute.StringSlice("response.tool_calls", toolNames))
		}()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			chunk := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), chunkPrefix))
			setRequestToFirstTokenTime(span)

			if len(chunk) == 0 {
				continue
			}

			if chunk == endMessage {
				break
			}

			var responseBody streamingResponseBody
			err := json.Unmarshal([]byte(chunk), &responseBody)
			if err != nil {
				err = fmt.Errorf("error unmarshalling JSON: %w", err)
				span.RecordError(err)
				if !yield(nil, err) {
					return
				}
				continue
			}
			var finishReason *string
			if len(responseBody.Choices) > 0 {
				delta := responseBody.Choices[0].Delta

				if delta.FinishReason != nil {
					finishReason = delta.FinishReason
				}

				if len(delta.ToolCalls) > 0 {
					toolCalls = append(toolCalls, delta.ToolCalls...)
					for _, toolCall := range delta.ToolCalls {
						if !yield(StreamToolCallChunk{
							finishReason: finishReason,
							toolCall: llms.ToolCall{
								ID:        toolCall.ID,
								Type:      toolCall.Type,
								Name:      toolCall.Function.Name,
								Arguments: toolCall.Function.Arguments,
								Function: llms.ToolCallFunction{
									Name:      toolCall.Function.Name,
									Arguments: toolCall.Function.Arguments,
								},
							},
						}, nil) {
							return
						}
					}
				}

				if delta.Content != "" {
					content := delta.Content
					if !yield(StreamContentChunk{
						finishReason: finishReason,
						content:      content,
					}, nil) {
						return
					}
				}

				if delta.Reasoning != "" {
					reasoning := delta.Reasoning
					if !yield(StreamReasoningChunk{
						finishReason: finishReason,
						reasoning:    reasoning,
						channel:      delta.Channel,
					}, nil) {
						return
					}
				}
			}

			if responseBody.Usage != nil {
				span.SetAttributes(attribute.Int("usage.input", responseBody.Usage.PromptTokens))
				span.SetAttributes(attribute.Int("usage.prompt", responseBody.Usage.PromptTokens))
				span.SetAttributes(attribute.Int("usage.output", responseBody.Usage.CompletionTokens))
				span.SetAttributes(attribute.Int("usage.completion", responseBody.Usage.CompletionTokens))
				span.SetAttributes(attribute.Int("usage.total", responseBody.Usage.TotalTokens))

				span.SetAttributes(attribute.Float64("usage.queue_time", responseBody.Usage.QueueTime))
				span.SetAttributes(attribute.Float64("usage.prompt_time", responseBody.Usage.PromptTime))
				span.SetAttributes(attribute.Float64("usage.completion_time", responseBody.Usage.CompletionTime))
				span.SetAttributes(attribute.Float64("usage.total_time", responseBody.Usage.TotalTime))

				var outputTokensDetails *llms.OutputTokensDetails
				var completionTokensDetails *llms.CompletionTokensDetails
				if responseBody.Usage.CompletionTokensDetails != nil {
					span.SetAttributes(attribute.Int("usage.reasoning", responseBody.Usage.CompletionTokensDetails.ReasoningTokens))
					completionTokensDetails = utils.Ptr(llms.CompletionTokensDetails{
						ReasoningTokens: responseBody.Usage.CompletionTokensDetails.ReasoningTokens,
					})
					outputTokensDetails = utils.Ptr(llms.OutputTokensDetails{
						ReasoningTokens: responseBody.Usage.CompletionTokensDetails.ReasoningTokens,
					})
				}

				if !yield(StreamUsageChunk{
					finishReason: finishReason,
					usage: llms.Usage{
						InputTokens:             responseBody.Usage.PromptTokens,
						PromptTokens:            responseBody.Usage.PromptTokens,
						OutputTokens:            responseBody.Usage.CompletionTokens,
						CompletionTokens:        responseBody.Usage.CompletionTokens,
						CompletionTokensDetails: completionTokensDetails,
						OutputTokensDetails:     outputTokensDetails,
						TotalTokens:             responseBody.Usage.TotalTokens,

						QueueTime:      responseBody.Usage.QueueTime,
						PromptTime:     responseBody.Usage.PromptTime,
						CompletionTime: responseBody.Usage.CompletionTime,
						TotalTime:      responseBody.Usage.TotalTime,
					},
				}, nil) {
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
