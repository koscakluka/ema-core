package events

const (
	// KindToolCallStarted identifies tool call execution start.
	KindToolCallStarted Kind = "tool_call.started"
	// KindToolCallCompleted identifies successful tool call completion.
	KindToolCallCompleted Kind = "tool_call.completed"
	// KindToolCallFailed identifies tool call failure.
	KindToolCallFailed Kind = "tool_call.failed"
)

// ToolCallStarted marks start of tool execution.
type ToolCallStarted struct {
	Base
	ID        string
	Name      string
	Arguments string
}

// NewToolCallStarted creates a tool call started event.
func NewToolCallStarted(id, name, arguments string) ToolCallStarted {
	return ToolCallStarted{Base: NewBase(KindToolCallStarted), ID: id, Name: name, Arguments: arguments}
}

// ToolCallCompleted marks successful tool execution.
type ToolCallCompleted struct {
	Base
	ID       string
	Name     string
	Response string
}

// NewToolCallCompleted creates a tool call completed event.
func NewToolCallCompleted(id, name, response string) ToolCallCompleted {
	return ToolCallCompleted{Base: NewBase(KindToolCallCompleted), ID: id, Name: name, Response: response}
}

// ToolCallFailed marks failed tool execution.
type ToolCallFailed struct {
	Base
	ID    string
	Name  string
	Error string
}

// NewToolCallFailed creates a tool call failed event.
func NewToolCallFailed(id, name, err string) ToolCallFailed {
	return ToolCallFailed{Base: NewBase(KindToolCallFailed), ID: id, Name: name, Error: err}
}
