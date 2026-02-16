package llms

import "fmt"

// Message is a single message in a conversation, but actually it represents a
// response from an LLM. It is an alias for Response for backwards compatibility.
//
// Deprecated: Use Response instead
type Message Response

// Response is a single response from an LLM
type Response struct {
	Content   string
	ToolCalls []ToolCall

	// ToolCallID is the ID of the tool call that this response is responding to
	//
	// Deprecated: LLM should never respond to a tool call, this is only here
	// for backwards compatibility
	ToolCallID string
	// Role describes who the response (previously Message) is from
	//
	// Deprecated: Response always comes from the assistant, so the role would
	// always be the same assistant
	Role MessageRole
}

// Turn is a single turn taken in the conversation.
//
// Deprecated: (since v0.0.15) use TurnV1 instead
type Turn struct {
	Role TurnRole

	// Content is the content of the turn
	// In user's turn it is the prompt,
	// in assistant's turn it is the response
	Content   string
	ToolCalls []ToolCall

	Cancelled     bool
	Stage         TurnStage
	Interruptions []InterruptionV0

	// ToolCallID is the ID of the tool call that this turn is responding to
	//
	// Deprecated: The response is now a ToolCall property, this is only here
	// for backwards compatibility
	ToolCallID string
}

type TurnV1 struct {
	ID string
	// Event is what initiated the turn, e.g. a user message, notification,
	// completed tool call, etc.
	Event EventV0

	// Responses is a list of responses that the assistant has generated for
	// the turn. The assistant may generate multiple responses for a single
	// turn e.g. if the tool call is slow, or there is an error, there might,
	// be an intermediate response.
	Responses []TurnResponseV0
	// ToolCalls is a list of tool calls that were executed during the turn.
	ToolCalls []ToolCall
	// Interruptions is a list of interruptions that were triggered during the
	// turn.
	Interruptions []InterruptionV0

	// Finalized is true if the assistant has finalized the turn, i.e. the
	// assistant has generated a response and the assistant has finished
	// generating responses for the turn.
	IsFinalised bool
}

func (t *TurnV1) IsCancelled() bool {
	if !t.IsFinalised {
		return false
	}

	for _, response := range t.Responses {
		if !response.IsCompleted() {
			return true
		}
	}
	return false
}

func (t *TurnV1) HasAssistantPart() bool {
	return len(t.Responses) > 0 || len(t.ToolCalls) > 0 || len(t.Interruptions) > 0
}

type EventV0 interface {
	fmt.Stringer
}

type TurnResponseV0 struct {
	Message        string
	TypedMessage   string
	SpokenResponse string

	IsMessageFullyGenerated bool
	IsTyped                 bool
	IsSpoken                bool
}

func (g *TurnResponseV0) IsCompleted() bool {
	return g.IsMessageFullyGenerated &&
		(!g.IsTyped || len(g.Message) == len(g.TypedMessage)) &&
		(!g.IsSpoken || len(g.Message) == len(g.SpokenResponse))
}

func (g *TurnResponseV0) IsFullyTyped() bool {
	return g.IsMessageFullyGenerated && (!g.IsTyped || len(g.Message) == len(g.TypedMessage))
}

func (g *TurnResponseV0) IsFullySpoken() bool {
	return g.IsMessageFullyGenerated && (!g.IsSpoken || len(g.Message) == len(g.SpokenResponse))
}

type InterruptionV0 struct {
	ID       int64
	Type     string
	Source   string
	Resolved bool
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
	Response  string

	// Type is the type of tool call, e.g. function call
	//
	// Deprecated: All tool calls are function calls for us, no need to specify
	Type string
	// Function is the description of the tool call
	//
	// Deprecated: Use ToolCall Name and Arguments properties instead
	Function ToolCallFunction
}

// ToolCallFunction is a description of a tool call
//
// Deprecated: Use ToolCall Name and Arguments properties instead
type ToolCallFunction struct {
	Name      string
	Arguments string
}

// MessageRole describes who is the message from
//
// Deprecated: This is kept for backwards compatibility, but it will not be
// used anymore, llms should generate their own messages and message roles
// based on Turns content
type MessageRole string

const (
	MessageRoleSystem    MessageRole = "system"
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
	MessageRoleTool      MessageRole = "tool"
)

type TurnRole string

const (
	TurnRoleUser      TurnRole = "user"
	TurnRoleAssistant TurnRole = "assistant"
)

type TurnStage string

const (
	TurnStagePreparing          TurnStage = "preparing"
	TurnStageGeneratingResponse TurnStage = "generating_response"
	TurnStageSpeaking           TurnStage = "speaking"
	TurnStageFinalized          TurnStage = "finalized"
)
