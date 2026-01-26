package llms

import "context"

type Stream interface {
	Chunks(context.Context) func(func(StreamChunk, error) bool)
}

type StreamChunk interface {
	// ID() string
	// Object() string
	// Created() int
	// Model() string
	// SystemFingerprint() string
	FinishReason() *string
}

type StreamRoleChunk interface {
	StreamChunk
	Role() string
}

type StreamReasoningChunk interface {
	StreamChunk
	Reasoning() string
	Channel() string
}

type StreamContentChunk interface {
	StreamChunk
	Content() string
}

type StreamToolCallChunk interface {
	StreamChunk
	ToolCall() ToolCall
}

type StreamUsageChunk interface {
	StreamChunk
	Usage() Usage
}

// TODO: See if this actually makes any sense
// type choiceBase struct {
// 	Index int
// 	// Logprobs any
// 	FinishReason *string
// }

type Usage struct {
	// InputTokens represents the number of input tokens.
	InputTokens int
	// PromptTokens represents the number of input tokens.
	//
	// Deprecated: Alias for InputTokens - use InputTokens instead.
	PromptTokens int
	// InputTokensDetails represents a detailed breakdown of the input tokens.
	InputTokensDetails *InputTokensDetails
	// OutputTokens represents the number of output tokens.
	OutputTokens int
	// CompletionTokens represents the number of output tokens.
	//
	// Deprecated: Alias for OutputTokens - use OutputTokens instead.
	CompletionTokens int
	// OutputTokensDetails represents a detailed breakdown of the output tokens.
	OutputTokensDetails *OutputTokensDetails
	// CompletionTokensDetails represents a detailed breakdown of the output tokens.
	//
	// Deprecated: Alias for OutputTokensDetails - use OutputTokensDetails instead.
	CompletionTokensDetails *CompletionTokensDetails
	// TotalTokens represents the total number of tokens used.
	TotalTokens int

	// QueueTime represents the time it took to queue the request.
	//
	// Note: This might be just an approximation.
	QueueTime float64
	// InputProcessingTimes represents the time it took to process the input.
	//
	// Note: This might be just an approximation.
	InputProcessingTimes float64
	// PromptTime represents the time it took to prompt the request.
	//
	// Deprecated: Use InputProcessingTime instead.
	// Note: This might be just an approximation.
	PromptTime float64
	// OutputProcessingTime represents the time it took to process the output.
	//
	// Depricated: Use OutputProcessingTimes instead.
	// Note: This might be just an approximation.
	OutputProcessingTime float64
	// CompletionTime represents the time it took to complete the request.
	//
	// Note: This might be just an approximation.
	CompletionTime float64
	// TotalTime represents the total time it took to complete the request.
	//
	// Note: This might be just an approximation.
	TotalTime float64
}

// InputTokensDetails represents a detailed breakdown of the input tokens.
type InputTokensDetails struct {
	// CachedTokens represents the number of tokens that were retrieved from the
	// cache.
	CachedTokens int
}

// OutputTokensDetails represents a detailed breakdown of the output tokens.
type OutputTokensDetails struct {
	// ReasoningTokens represents the number of reasoning tokens.
	ReasoningTokens int
}

// CompletionTokensDetails represents a detailed breakdown of the output tokens.
//
// Deprecated: Use OutputTokensDetails instead.
type CompletionTokensDetails struct {
	// ReasoningTokens represents the number of reasoning tokens.
	ReasoningTokens int
}
