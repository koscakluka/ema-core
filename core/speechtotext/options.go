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

// WithTranscriptionCallback sets the callback to be invoked when the
// transcription is complete and the final result is available (usually after
// a grace period to confirm the end of speech).
//
// The transcript will include the whole transcription since the start
// of speech.
func WithTranscriptionCallback(callback func(transcript string)) TranscriptionOption {
	return func(o *TranscriptionOptions) {
		o.TranscriptionCallback = callback
	}
}

// WithPartialTranscriptionCallback sets the callback to be invoked when a part
// of the transcription is finalized (it will not be changed).
//
// The transcript will include only the latest finalized part of the
// transcription.
func WithPartialTranscriptionCallback(callback func(transcript string)) TranscriptionOption {
	return func(o *TranscriptionOptions) {
		o.PartialTranscriptionCallback = callback
	}
}

// WithSpeechStartedCallback sets the callback to be invoked when speech
// starts.
//
// The callback will be invoked immediately after the start of speech. It might
// be invoked multiple times without invoking the speech-ended callback.
func WithSpeechStartedCallback(callback func()) TranscriptionOption {
	return func(o *TranscriptionOptions) {
		o.SpeechStartedCallback = callback
	}
}

// WithSpeechEndedCallback sets the callback to be invoked when speech ends
// (usually after a grace period to confirm the end of speech).
func WithSpeechEndedCallback(callback func()) TranscriptionOption {
	return func(o *TranscriptionOptions) {
		o.SpeechEndedCallback = callback
	}
}

// WithPartialInterimTranscriptionCallback sets the callback to be invoked when
// a non-finalized part of the transcription is available. Future invocations
// might include the same (but possibly updated) part of the transcription.
//
// The transcript will include only the latest portion of the non-finalized
// transcription.
func WithPartialInterimTranscriptionCallback(callback func(transcript string)) TranscriptionOption {
	return func(o *TranscriptionOptions) {
		o.PartialInterimTranscriptionCallback = callback
	}
}

// WithInterimTranscriptionCallback sets the callback to be invoked when a
// non-finalized part of the transcription is available. Future invocations
// might update the latest (non-finalized) portion of the transcription.
//
// The transcript will include the whole transcription since the start
// of speech.
func WithInterimTranscriptionCallback(callback func(transcript string)) TranscriptionOption {
	return func(o *TranscriptionOptions) {
		o.InterimTranscriptionCallback = callback
	}
}

// WithEncodingInfo sets the encoding info to be used for transcription.
//
// Some speech-to-text implementations might require a specific encoding
// combinations, and might not support all encodings.
func WithEncodingInfo(encodingInfo audio.EncodingInfo) TranscriptionOption {
	return func(o *TranscriptionOptions) {
		o.EncodingInfo = encodingInfo
	}
}
