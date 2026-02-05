package speechtotext

import "github.com/koscakluka/ema-core/core/audio"

type TranscriptionOptions struct {
	PartialInterimTranscriptionCallback func(transcript string)
	InterimTranscriptionCallback        func(transcript string)
	PartialTranscriptionCallback        func(transcript string)
	TranscriptionCallback               func(transcript string)

	SpeechStartedCallback func()
	SpeechEndedCallback   func()

	EncodingInfo audio.EncodingInfo
}

type TranscriptionOption func(*TranscriptionOptions)

func WithTranscriptionCallback(callback func(transcript string)) TranscriptionOption {
	return func(o *TranscriptionOptions) {
		o.TranscriptionCallback = callback
	}
}

func WithPartialTranscriptionCallback(callback func(transcript string)) TranscriptionOption {
	return func(o *TranscriptionOptions) {
		o.PartialTranscriptionCallback = callback
	}
}

func WithSpeechStartedCallback(callback func()) TranscriptionOption {
	return func(o *TranscriptionOptions) {
		o.SpeechStartedCallback = callback
	}
}

func WithSpeechEndedCallback(callback func()) TranscriptionOption {
	return func(o *TranscriptionOptions) {
		o.SpeechEndedCallback = callback
	}
}

func WithPartialInterimTranscriptionCallback(callback func(transcript string)) TranscriptionOption {
	return func(o *TranscriptionOptions) {
		o.PartialInterimTranscriptionCallback = callback
	}
}

func WithInterimTranscriptionCallback(callback func(transcript string)) TranscriptionOption {
	return func(o *TranscriptionOptions) {
		o.InterimTranscriptionCallback = callback
	}
}

func WithEncodingInfo(encodingInfo audio.EncodingInfo) TranscriptionOption {
	return func(o *TranscriptionOptions) {
		o.EncodingInfo = encodingInfo
	}
}
