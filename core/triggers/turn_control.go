package triggers

type CancelTurnTrigger struct{ BaseTrigger }

func (e CancelTurnTrigger) String() string { return "cancel turn" }

func NewCancelTurnTrigger(opts ...RebaseOption) CancelTurnTrigger {
	base := NewBaseTrigger()
	for _, opt := range opts {
		opt(&base)
	}

	return CancelTurnTrigger{BaseTrigger: base}
}

type PauseTurnTrigger struct{ BaseTrigger }

func (e PauseTurnTrigger) String() string { return "pause turn" }

func NewPauseTurnTrigger(opts ...RebaseOption) PauseTurnTrigger {
	base := NewBaseTrigger()
	for _, opt := range opts {
		opt(&base)
	}

	return PauseTurnTrigger{BaseTrigger: base}
}

type UnpauseTurnTrigger struct{ BaseTrigger }

func (e UnpauseTurnTrigger) String() string { return "unpause turn" }

func NewUnpauseTurnTrigger(opts ...RebaseOption) UnpauseTurnTrigger {
	base := NewBaseTrigger()
	for _, opt := range opts {
		opt(&base)
	}

	return UnpauseTurnTrigger{BaseTrigger: base}
}
