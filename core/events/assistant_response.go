package events

const (
	// KindAssistantResponseSegment identifies streamed assistant response text.
	KindAssistantResponseSegment Kind = "assistant_response.segment"
	// KindAssistantResponseFinal identifies assistant response stream completion.
	KindAssistantResponseFinal Kind = "assistant_response.final"
)

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
