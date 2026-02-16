package events

import "time"

type BaseEvent struct {
	timestamp time.Time
}

func NewBaseEvent() BaseEvent {
	return BaseEvent{timestamp: time.Now()}
}

func (t BaseEvent) Timestamp() time.Time {
	return t.timestamp
}

type RebaseOption func(*BaseEvent)

func WithBase(base BaseEvent) RebaseOption {
	return func(o *BaseEvent) {
		*o = base
	}
}
