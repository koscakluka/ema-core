package events

import "github.com/koscakluka/ema-core/core/llms"

type RecordInterruptionEvent struct {
	BaseEvent
	Interruption llms.InterruptionV0
}

func (e RecordInterruptionEvent) String() string { return "record interruption" }

func NewRecordInterruptionEvent(interruption llms.InterruptionV0, opts ...RebaseOption) RecordInterruptionEvent {
	base := NewBaseEvent()
	for _, opt := range opts {
		opt(&base)
	}

	return RecordInterruptionEvent{BaseEvent: base, Interruption: interruption}
}

type ResolveInterruptionEvent struct {
	BaseEvent
	ID       int64
	Type     string
	Resolved bool
}

func (e ResolveInterruptionEvent) String() string { return "resolve interruption" }

func NewResolveInterruptionEvent(id int64, typ string, resolved bool, opts ...RebaseOption) ResolveInterruptionEvent {
	base := NewBaseEvent()
	for _, opt := range opts {
		opt(&base)
	}

	return ResolveInterruptionEvent{
		BaseEvent: base,
		ID:        id,
		Type:      typ,
		Resolved:  resolved,
	}
}
