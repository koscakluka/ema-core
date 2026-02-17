package interruptions

type interruptionType string

const (
	InterruptionTypeContinuation  interruptionType = "continuation"
	InterruptionTypeClarification interruptionType = "clarification"
	InterruptionTypeCancellation  interruptionType = "cancellation"
	InterruptionTypeIgnorable     interruptionType = "ignorable"
	InterruptionTypeRepetition    interruptionType = "repetition"
	InterruptionTypeNoise         interruptionType = "noise"
	InterruptionTypeAction        interruptionType = "action"
	InterruptionTypeNewPrompt     interruptionType = "new prompt"
)
