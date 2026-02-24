package events

const (
	// KindAssistantResponseStarted identifies assistant response generation start.
	KindAssistantResponseStarted Kind = "assistant_response.started"
	// KindAssistantResponseSegment identifies streamed assistant response text.
	KindAssistantResponseSegment Kind = "assistant_response.segment"
	// KindAssistantResponseFinal identifies assistant response stream completion.
	KindAssistantResponseFinal Kind = "assistant_response.final"
	// KindAssistantResponseFinalized identifies final assembled assistant response payload.
	KindAssistantResponseFinalized Kind = "assistant_response.finalized"
)

// AssistantResponseStarted marks assistant response generation start.
type AssistantResponseStarted struct{ Base }

// NewAssistantResponseStarted creates an assistant response started event.
func NewAssistantResponseStarted() AssistantResponseStarted {
	return AssistantResponseStarted{Base: NewBase(KindAssistantResponseStarted)}
}

// AssistantResponseSegment carries a streamed assistant response text segment.
type AssistantResponseSegment struct {
	Base
	Segment string
}

// NewAssistantResponseSegment creates an assistant response segment event.
func NewAssistantResponseSegment(segment string) AssistantResponseSegment {
	return AssistantResponseSegment{Base: NewBase(KindAssistantResponseSegment), Segment: segment}
}

// AssistantResponseFinal marks assistant response stream completion.
type AssistantResponseFinal struct{ Base }

// NewAssistantResponseFinal creates an assistant response final event.
func NewAssistantResponseFinal() AssistantResponseFinal {
	return AssistantResponseFinal{Base: NewBase(KindAssistantResponseFinal)}
}

// AssistantResponseFinalized carries the final assembled assistant response.
type AssistantResponseFinalized struct {
	Base
	Response string
}

// NewAssistantResponseFinalized creates an assistant response finalized event.
func NewAssistantResponseFinalized(response string) AssistantResponseFinalized {
	return AssistantResponseFinalized{Base: NewBase(KindAssistantResponseFinalized), Response: response}
}
