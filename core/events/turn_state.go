package events

const (
	// KindTurnStarted identifies turn start.
	KindTurnStarted Kind = "turn_state.started"
	// KindTurnCompleted identifies successful turn completion.
	KindTurnCompleted Kind = "turn_state.completed"
	// KindTurnFailed identifies turn failure.
	KindTurnFailed Kind = "turn_state.failed"
	// KindTurnCancelled identifies turn cancellation.
	KindTurnCancelled Kind = "turn_state.cancelled"
)

// TurnStarted marks creation of a new turn.
type TurnStarted struct {
	Base
	TurnID  string
	Trigger string
}

// NewTurnStarted creates a turn started event.
func NewTurnStarted(turnID, trigger string) TurnStarted {
	return TurnStarted{Base: NewBase(KindTurnStarted), TurnID: turnID, Trigger: trigger}
}

// TurnCompleted marks successful completion of a turn.
type TurnCompleted struct {
	Base
	TurnID string
}

// NewTurnCompleted creates a turn completed event.
func NewTurnCompleted(turnID string) TurnCompleted {
	return TurnCompleted{Base: NewBase(KindTurnCompleted), TurnID: turnID}
}

// TurnFailed marks failure of a turn.
type TurnFailed struct {
	Base
	TurnID string
	Error  string
}

// NewTurnFailed creates a turn failed event.
func NewTurnFailed(turnID, err string) TurnFailed {
	return TurnFailed{Base: NewBase(KindTurnFailed), TurnID: turnID, Error: err}
}

// TurnCancelled marks cancellation of the current turn.
type TurnCancelled struct{ Base }

// NewTurnCancelled creates a turn cancelled event.
func NewTurnCancelled() TurnCancelled {
	return TurnCancelled{Base: NewBase(KindTurnCancelled)}
}
