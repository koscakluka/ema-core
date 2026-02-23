package triggers

import "github.com/koscakluka/ema-core/core/llms"

type RecordInterruptionTrigger struct {
	BaseTrigger
	Interruption llms.InterruptionV0
}

func (e RecordInterruptionTrigger) String() string { return "record interruption" }

func NewRecordInterruptionTrigger(interruption llms.InterruptionV0, opts ...RebaseOption) RecordInterruptionTrigger {
	base := NewBaseTrigger()
	for _, opt := range opts {
		opt(&base)
	}

	return RecordInterruptionTrigger{BaseTrigger: base, Interruption: interruption}
}

type ResolveInterruptionTrigger struct {
	BaseTrigger
	ID       int64
	Type     string
	Resolved bool
}

func (e ResolveInterruptionTrigger) String() string { return "resolve interruption" }

func NewResolveInterruptionTrigger(id int64, typ string, resolved bool, opts ...RebaseOption) ResolveInterruptionTrigger {
	base := NewBaseTrigger()
	for _, opt := range opts {
		opt(&base)
	}

	return ResolveInterruptionTrigger{
		BaseTrigger: base,
		ID:          id,
		Type:        typ,
		Resolved:    resolved,
	}
}
