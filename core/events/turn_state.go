package events

// KindTurnCancelled identifies turn cancellation.
const KindTurnCancelled Kind = "turn_state.cancelled"

// TurnCancelled marks cancellation of the current turn.
type TurnCancelled struct{ Base }

// NewTurnCancelled creates a turn cancelled event.
func NewTurnCancelled() TurnCancelled {
	return TurnCancelled{Base: NewBase(KindTurnCancelled)}
}
