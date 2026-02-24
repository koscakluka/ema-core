package events

const (
	// KindAssistantSpeechFrame identifies synthesized assistant speech audio.
	KindAssistantSpeechFrame Kind = "assistant_speech.frame"
	// KindAssistantSpeechMarkGenerated identifies a generated TTS mark.
	KindAssistantSpeechMarkGenerated Kind = "assistant_speech.mark_generated"
	// KindAssistantSpeechFinal identifies TTS generation completion.
	KindAssistantSpeechFinal Kind = "assistant_speech.final"
)

// AssistantSpeechFrame carries a synthesized assistant speech audio frame.
type AssistantSpeechFrame struct {
	Base
	Audio []byte
}

// NewAssistantSpeechFrame creates an assistant speech audio frame event.
func NewAssistantSpeechFrame(audio []byte) AssistantSpeechFrame {
	return AssistantSpeechFrame{Base: NewBase(KindAssistantSpeechFrame), Audio: audio}
}

// AssistantSpeechMarkGenerated carries transcript text attached to a generated TTS mark.
type AssistantSpeechMarkGenerated struct {
	Base
	Transcript string
}

// NewAssistantSpeechMarkGenerated creates an assistant speech mark generated event.
func NewAssistantSpeechMarkGenerated(transcript string) AssistantSpeechMarkGenerated {
	return AssistantSpeechMarkGenerated{Base: NewBase(KindAssistantSpeechMarkGenerated), Transcript: transcript}
}

// AssistantSpeechFinal marks completion of TTS generation.
type AssistantSpeechFinal struct{ Base }

// NewAssistantSpeechFinal creates an assistant speech final event.
func NewAssistantSpeechFinal() AssistantSpeechFinal {
	return AssistantSpeechFinal{Base: NewBase(KindAssistantSpeechFinal)}
}
