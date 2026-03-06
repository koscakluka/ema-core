package soniox

// Model is a Soniox real-time STT model identifier.
type Model string

const defaultModel = ModelSTTRTV4

const (
	ModelSTTRTV4 Model = "stt-rt-v4"

	// Deprecated: All request are automatically routed to [ModelSTTRTV4].
	ModelSTTRTV3 Model = "stt-rt-v3"
)
