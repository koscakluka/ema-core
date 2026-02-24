package events

import "time"

type Kind string

type Event interface {
	Kind() Kind
	Timestamp() time.Time
}

type Base struct {
	kind      Kind
	timestamp time.Time
}

func NewBase(kind Kind) Base {
	return Base{kind: kind, timestamp: time.Now()}
}

func (b Base) Kind() Kind {
	return b.kind
}

func (b Base) Timestamp() time.Time {
	return b.timestamp
}
