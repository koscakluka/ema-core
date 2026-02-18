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

type PauseTurnEvent struct{ BaseEvent }

func (e PauseTurnEvent) String() string { return "pause turn" }

func NewPauseTurnEvent(opts ...RebaseOption) PauseTurnEvent {
	base := NewBaseEvent()
	for _, opt := range opts {
		opt(&base)
	}

	return PauseTurnEvent{BaseEvent: base}
}

type UnpauseTurnEvent struct{ BaseEvent }

func (e UnpauseTurnEvent) String() string { return "unpause turn" }

func NewUnpauseTurnEvent(opts ...RebaseOption) UnpauseTurnEvent {
	base := NewBaseEvent()
	for _, opt := range opts {
		opt(&base)
	}

	return UnpauseTurnEvent{BaseEvent: base}
}
