package events

type CancelTurnEvent struct{ BaseEvent }

func (e CancelTurnEvent) String() string { return "cancel turn" }

func NewCancelTurnEvent(opts ...RebaseOption) CancelTurnEvent {
	base := NewBaseEvent()
	for _, opt := range opts {
		opt(&base)
	}

	return CancelTurnEvent{BaseEvent: base}
}
