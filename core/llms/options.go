package llms

import (
	"slices"
	"strings"
)

// PromptOptions is a struct that contains all the options for a prompt. It is
// used as a base for both general and streaming prompt options.
//
// Deprecated: this struct will be removed and replaced with a more specific
// option patterns
type PromptOptions struct {
	Instructions    string
	Turns           []Turn
	TurnsV1         []TurnV1
	Messages        []Message
	Stream          func(string)
	Tools           []Tool
	ForcedToolsCall bool
}

type BaseOptions struct {
	Instructions string
	Messages     []Message
	Turns        []Turn
	TurnsV1      []TurnV1
}

type GeneralPromptOptions struct {
	BaseOptions
	PromptOptions
	Tools           []Tool
	ForcedToolsCall bool
}

type StreamingPromptOptions struct {
	GeneralPromptOptions
}

type StructuredPromptOptions struct {
	BaseOptions
	PromptOptions
}

// PromptOption is a function that can be used to modify the prompt options.
//
// Deprecated: this type will be removed and replaced with a more specific
// option patterns
type PromptOption func(*PromptOptions)

type GeneralPromptOption interface {
	ApplyToGeneral(*GeneralPromptOptions)
}

type StreamingPromptOption interface {
	ApplyToStreaming(*StreamingPromptOptions)
}

type StructuredPromptOption interface {
	ApplyToStructured(*StructuredPromptOptions)
}

func (f PromptOption) ApplyToGeneral(o *GeneralPromptOptions) {
	o.PromptOptions.Messages = o.BaseOptions.Messages
	o.PromptOptions.Turns = o.BaseOptions.Turns
	o.PromptOptions.Tools = o.Tools
	o.PromptOptions.ForcedToolsCall = o.ForcedToolsCall
	o.PromptOptions.Instructions = o.BaseOptions.Instructions
	o.PromptOptions.TurnsV1 = o.BaseOptions.TurnsV1
	f(&o.PromptOptions)
	o.BaseOptions.TurnsV1 = o.PromptOptions.TurnsV1
	o.BaseOptions.Instructions = o.PromptOptions.Instructions
	o.BaseOptions.Messages = o.PromptOptions.Messages
	o.BaseOptions.Turns = o.PromptOptions.Turns
	o.Tools = o.PromptOptions.Tools
	o.ForcedToolsCall = o.PromptOptions.ForcedToolsCall
}

func (f PromptOption) ApplyToStreaming(o *StreamingPromptOptions) {
	o.PromptOptions.Messages = o.GeneralPromptOptions.BaseOptions.Messages
	o.PromptOptions.Turns = o.GeneralPromptOptions.BaseOptions.Turns
	o.PromptOptions.Tools = o.GeneralPromptOptions.Tools
	o.PromptOptions.ForcedToolsCall = o.GeneralPromptOptions.ForcedToolsCall
	o.PromptOptions.Instructions = o.BaseOptions.Instructions
	o.PromptOptions.TurnsV1 = o.BaseOptions.TurnsV1
	f(&o.PromptOptions)
	o.BaseOptions.TurnsV1 = o.PromptOptions.TurnsV1
	o.BaseOptions.Instructions = o.PromptOptions.Instructions
	o.BaseOptions.Messages = o.PromptOptions.Messages
	o.BaseOptions.Turns = o.PromptOptions.Turns
	o.GeneralPromptOptions.Tools = o.PromptOptions.Tools
	o.GeneralPromptOptions.ForcedToolsCall = o.PromptOptions.ForcedToolsCall
}

func (f PromptOption) ApplyToStructured(o *StructuredPromptOptions) {
	o.PromptOptions.Messages = o.BaseOptions.Messages
	o.PromptOptions.Turns = o.BaseOptions.Turns
	o.PromptOptions.TurnsV1 = o.BaseOptions.TurnsV1
	o.PromptOptions.Instructions = o.BaseOptions.Instructions
	f(&o.PromptOptions)
	o.BaseOptions.Instructions = o.PromptOptions.Instructions
	o.BaseOptions.TurnsV1 = o.PromptOptions.TurnsV1
	o.BaseOptions.Messages = o.PromptOptions.Messages
	o.BaseOptions.Turns = o.PromptOptions.Turns
}

// WithStream is a PromptOption that sets the stream callback for the prompt.
//
// Deprecated: Use specialized streaming method instead of general one
func WithStream(stream func(string)) PromptOption {
	return func(opts *PromptOptions) {
		opts.Stream = stream
	}
}

// WithSystemPrompt is a PromptOption that sets the system prompt for the
// prompt.
// Repeating this option will overwrite the previous system prompt.
func WithSystemPrompt(prompt string) PromptOption {
	return func(opts *PromptOptions) {
		opts.Instructions = prompt
		if len(opts.Messages) == 0 {
			opts.Messages = append(opts.Messages, Message{
				Role:    MessageRoleSystem,
				Content: prompt,
			})
		} else if opts.Messages[0].Role == MessageRoleSystem {
			opts.Messages[0].Content = prompt
		} else {
			opts.Messages = append([]Message{{
				Role:    MessageRoleSystem,
				Content: prompt,
			}}, opts.Messages...)
		}
	}
}

// WithMessages is a PromptOption that adds passed messages to the prompt.
// Repeating this option will sequentially add more messages.
//
// Deprecated: Use WithTurns instead
func WithMessages(messages ...Message) PromptOption {
	return func(opts *PromptOptions) {
		opts.Messages = append(opts.Messages, messages...)
		opts.Turns = append(opts.Turns, ToTurns(messages)...)
		opts.TurnsV1 = append(opts.TurnsV1, ToTurnsV1FromV0(ToTurns(messages))...)
	}
}

// WithTurns is a PromptOption that adds turns information to the prompt.
// Repeating this option will sequentially add more turns.
func WithTurns(turns ...Turn) PromptOption {
	return func(opts *PromptOptions) {
		opts.Turns = append(opts.Turns, turns...)
		opts.Messages = append(opts.Messages, ToMessages(turns)...)
		opts.TurnsV1 = append(opts.TurnsV1, ToTurnsV1FromV0(turns)...)
	}
}

func WithTurnsV1(turns ...TurnV1) PromptOption {
	return func(opts *PromptOptions) {
		opts.TurnsV1 = append(opts.TurnsV1, turns...)
		opts.Turns = append(opts.Turns, ToTurnsV0FromV1(turns)...)
		opts.Messages = append(opts.Messages, ToMessages(opts.Turns)...)
	}
}

// WithTools is a PromptOption that adds tools to the prompt
//
// This option does nothing for structured prompts, it is depricated for use
// there and will be disabled in the future
func WithTools(tools ...Tool) PromptOption {
	return func(opts *PromptOptions) {
		opts.Tools = append(opts.Tools, tools...)
	}
}

// WithForcedTools is a PromptOption that forces the use of tools in the prompt.
// Note that any tool that is available can be used, not just the ones passed
// into this option.
//
// This option does nothing for structured prompts, it is depricated for use
// there and will be disabled in the future
func WithForcedTools(tools ...Tool) PromptOption {
	return func(opts *PromptOptions) {
		opts.Tools = tools
	}
}

func ToMessages(turns []Turn) []Message {
	var messages []Message
	for _, turn := range turns {
		switch turn.Role {
		case TurnRoleUser:
			messages = append(messages, Message{
				Role:    MessageRoleUser,
				Content: turn.Content,
			})
		case TurnRoleAssistant:
			if len(turn.ToolCalls) > 0 {
				msg := Message{Role: MessageRoleAssistant}
				responseMsgs := []Message{}
				for _, toolCall := range turn.ToolCalls {
					msg.ToolCalls = append(msg.ToolCalls, ToolCall{
						ID:   toolCall.ID,
						Type: toolCall.Type,
						Function: ToolCallFunction{
							Name:      toolCall.Name,
							Arguments: toolCall.Arguments,
						},
					})
					if toolCall.Response != "" {
						responseMsgs = append(responseMsgs, Message{
							Role:       MessageRoleTool,
							Content:    toolCall.Response,
							ToolCallID: toolCall.ID,
						})
					}
				}
				messages = append(messages, msg)
				messages = append(messages, responseMsgs...)
			}
			if len(turn.Content) > 0 {
				messages = append(messages, Message{
					Role:    MessageRoleAssistant,
					Content: turn.Content,
				})
			}
		}
	}
	return messages
}

func ToTurns(messages []Message) []Turn {
	turns := []Turn{}
	popLastTurn := func() Turn {
		if len(turns) == 0 {
			return Turn{}
		}
		turn := turns[len(turns)-1]
		turns = turns[:len(turns)-1]
		return turn
	}

	for _, message := range messages {
		turn := popLastTurn()
		switch message.Role {
		case MessageRoleSystem:
			// TODO: Technically, this should save the instructions
			// for the assistant turn, but it isn't actually implemented like
			// that anywhere so we can just skip it
		case MessageRoleUser:
			if turn.Role == TurnRoleAssistant {
				turns = append(turns, turn)
				turn = Turn{}
			}

			turn.Role = TurnRoleUser
			turn.Content = message.Content
			if turn.Content != "" {
				turns = append(turns, turn)
			}
		case MessageRoleAssistant:
			if turn.Role == TurnRoleUser {
				turns = append(turns, turn)
				turn = Turn{}
			}

			turn.Role = TurnRoleAssistant
			turn.Content = message.Content
			for _, toolCall := range message.ToolCalls {
				newToolCall := ToolCall{
					ID:        toolCall.ID,
					Type:      toolCall.Type,
					Name:      toolCall.Name,
					Arguments: toolCall.Arguments,
					Function: ToolCallFunction{
						Name:      toolCall.Function.Name,
						Arguments: toolCall.Function.Arguments,
					},
				}
				if newToolCall.Name == "" {
					newToolCall.Name = newToolCall.Function.Name
				}
				if newToolCall.Arguments == "" {
					newToolCall.Arguments = newToolCall.Function.Arguments
				}
				turn.ToolCalls = append(turn.ToolCalls, newToolCall)
			}
			turns = append(turns, turn)

		case MessageRoleTool:
			if turn.Role != "" {
				turns = append(turns, turn)
			}
			for i, turn := range slices.Backward(turns) {
				if turn.Role != TurnRoleAssistant {
					continue
				}
				toolIdx := slices.IndexFunc(turn.ToolCalls, func(t ToolCall) bool {
					return t.ID == message.ToolCallID
				})
				if toolIdx != -1 {
					turns[i].ToolCalls[toolIdx].Response = message.Content
					break
				}
			}

		}
	}
	return turns
}

func ToTurnsV1FromV0(turns []Turn) []TurnV1 {
	turnsV1 := []TurnV1{}
	var turnV1 *TurnV1
	for _, turn := range turns {
		switch turn.Role {
		case TurnRoleUser:
			if turnV1 != nil {
				turnsV1 = append(turnsV1, *turnV1)
			}
			turnV1 = &TurnV1{
				Trigger: UserPromptTrigger{Prompt: turn.Content},
			}
		case TurnRoleAssistant:
			turnV1.Interruptions = append(turnV1.Interruptions, turn.Interruptions...)
			turnV1.ToolCalls = append(turnV1.ToolCalls, turn.ToolCalls...)
			turnV1.Responses = append(turnV1.Responses, TurnResponseV0{
				Message:                 turn.Content,
				TypedMessage:            turn.Content,
				SpokenResponse:          turn.Content,
				IsMessageFullyGenerated: !turn.Cancelled,
				IsTyped:                 false,
				IsSpoken:                false,
			})
			turnV1.IsFinalised = turn.Stage == TurnStageFinalized
		}
	}

	if turnV1 != nil {
		turnsV1 = append(turnsV1, *turnV1)
	}
	return turnsV1
}

func ToTurnsV0FromV1(turns []TurnV1) []Turn {
	turnsV0 := []Turn{}
	for _, turn := range turns {
		turnsV0 = append(turnsV0, Turn{
			Role:    TurnRoleUser,
			Content: turn.Trigger.String(),
		})
		var message strings.Builder
		for _, response := range turn.Responses {
			if response.IsTyped {
				message.WriteString(response.TypedMessage)
			} else if response.IsSpoken {
				message.WriteString(response.SpokenResponse)
			} else {
				message.WriteString(response.Message)
			}
		}
		if turn.HasAssistantPart() {
			turnsV0 = append(turnsV0, Turn{
				Role:          TurnRoleAssistant,
				Content:       message.String(),
				ToolCalls:     turn.ToolCalls,
				Interruptions: turn.Interruptions,
			})
		}

	}
	return turnsV0
}
