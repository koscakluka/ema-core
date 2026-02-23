package triggers

import "time"

type BaseTrigger struct {
	timestamp time.Time
}

func NewBaseTrigger() BaseTrigger {
	return BaseTrigger{timestamp: time.Now()}
}

func (t BaseTrigger) Timestamp() time.Time {
	return t.timestamp
}

type RebaseOption func(*BaseTrigger)

func WithBase(base BaseTrigger) RebaseOption {
	return func(o *BaseTrigger) {
		*o = base
	}
}
