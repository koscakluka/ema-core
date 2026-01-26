package groq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"

	"github.com/invopop/jsonschema"
	"github.com/koscakluka/ema-core/core/llms"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
)

func PromptJSONSchema[T any](
	ctx context.Context,
	apiKey string,
	model string,
	prompt string,
	systemPrompt string,
	outputSchema T,
	opts ...llms.StructuredPromptOption,
) (*T, error) {
	ctx, span := tracer.Start(ctx, "prompt llm structured")
	defer span.End()

	options := llms.StructuredPromptOptions{
		BaseOptions: llms.BaseOptions{Instructions: systemPrompt},
	}
	for _, opt := range opts {
		opt.ApplyToStructured(&options)
	}

	messages := toMessages(options.BaseOptions.Instructions, options.BaseOptions.Turns)
	messages = append(messages, message{
		Role:    messageRoleUser,
		Content: prompt,
	})

	// TODO: Implement a custom reflector that only satisfies the subset of
	// jsonschema used by groq
	reflector := jsonschema.Reflector{DoNotReference: true}
	var (
		schema         *jsonschema.Schema
		outputTypeName string
	)
	if reflect.TypeOf(outputSchema).Kind() == reflect.Ptr {
		schema = reflector.ReflectFromType(reflect.TypeOf(outputSchema).Elem())
		outputTypeName = reflect.TypeOf(outputSchema).Elem().Name()
	} else {
		schema = reflector.Reflect(outputSchema)
		outputTypeName = reflect.TypeOf(outputSchema).Name()
	}

	reqBody := schemaRequestBody{
		Model:    model,
		Messages: messages,
		ResponseFormat: &ChatResponseFormat{
			Type: "json_schema",
			JSONSchema: &JSONSchema{
				Name: outputTypeName,
				// Description: schema.Description,
				Schema: *schema,
				Strict: true,
			},
		},
	}

	span.SetAttributes(attribute.String("request.model", model))
	schemaString, _ := schema.MarshalJSON()
	span.SetAttributes(attribute.String("request.schema", string(schemaString)))

	requestBodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		err = fmt.Errorf("error marshalling JSON: %w", err)
		span.RecordError(err)
		span.SetAttributes(attribute.String("error", err.Error()))
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestBodyBytes))
	if err != nil {
		err = fmt.Errorf("error creating HTTP request: %w", err)
		span.RecordError(err)
		span.SetAttributes(attribute.String("error", err.Error()))
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	span.SetAttributes(attribute.String("request.url", req.URL.String()))
	client := &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}
	resp, err := client.Do(req)
	if err != nil {
		err = fmt.Errorf("error sending request: %w", err)
		span.RecordError(err)
		span.SetAttributes(attribute.String("error", err.Error()))
		return nil, err
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
		span.SetAttributes(attribute.String("error", err.Error()))
		return nil, err
	}
	// response, err := c.ChatCompletion(ctx, request)
	// if err != nil {
	// 	reqErr, ok := err.(*groqerr.APIError)
	// 	if ok && (reqErr.HTTPStatusCode == http.StatusServiceUnavailable ||
	// 		reqErr.HTTPStatusCode == http.StatusInternalServerError) {
	// 		time.Sleep(request.RetryDelay)
	// 		return c.ChatCompletionJSON(ctx, request, output)
	// 	}
	// }

	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("error reading response body: %w", err)
		span.RecordError(err)
		span.SetAttributes(attribute.String("error", err.Error()))
		return nil, err
	}
	var responseBody schemaResponseBody
	err = json.Unmarshal(respBodyBytes, &responseBody)

	content := responseBody.Choices[0].Message.Content
	split := strings.Split(content, "```")
	if len(split) > 1 {
		content = split[1]
	}
	err = json.Unmarshal([]byte(content), outputSchema)
	if err != nil {
		err = fmt.Errorf("error unmarshalling response: %w", err)
		span.RecordError(err)
		span.SetAttributes(attribute.String("error", err.Error()))
		return nil, err
	}

	return &outputSchema, nil
}

type schemaRequestBody struct {
	Model          string              `json:"model"`
	Messages       []message           `json:"messages"`
	ResponseFormat *ChatResponseFormat `json:"response_format,omitempty"`
}

type ChatResponseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

type JSONSchema struct {
	// Name is the name of the chat completion response format json
	// schema.
	//
	// it is used to further identify the schema in the response.
	Name string `json:"name"`
	// Description is the description of the chat completion
	// response format json schema.
	Description string `json:"description,omitempty"`
	// Schema is the schema of the chat completion response format
	// json schema.
	Schema jsonschema.Schema `json:"schema"`
	// Strict determines whether to enforce the schema upon the
	// generated content.
	Strict bool `json:"strict"`
}

type schemaResponseBody struct {
	Choices []struct {
		Message struct {
			Role         string     `json:"role,omitempty"`
			Content      string     `json:"content,omitempty"`
			ToolCalls    []toolCall `json:"tool_calls,omitempty"`
			Reasoning    string     `json:"reasoning,omitempty"`
			Channel      string     `json:"channel,omitempty"`
			FinishReason *string    `json:"finish_reason,omitempty"`
		} `json:"message"`
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
