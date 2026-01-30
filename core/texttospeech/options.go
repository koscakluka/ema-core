package texttospeech

import "github.com/koscakluka/ema-core/core/audio"

type TextToSpeechOptions struct {
	// AudioCallback is called when the TTS client produces audio
	//
	// Deprecated: (since v0.0.14) use SpeechAudioCallback instead
	AudioCallback func(audio []byte)
	// SpeechAudioCallback is called when the TTS client produces audio
	SpeechAudioCallback func(audio []byte)
	// AudioEnded is called when the TTS client has finished producing audio
	//
	// Deprecated: (since v0.0.14) use SpeechMarkCallback instead
	AudioEnded func(transcript string)
	// SpeechMarkCallback is called when the TTS client produces speech until the
	// marked text. Each mark is called once.
	SpeechMarkCallback func(string)
	// SpeechEndedCallbackV0 is called when the TTS client has finished producing speech
	// and provides a report of the speech generation
	SpeechEndedCallbackV0 func(SpeechEndedReport)
	// ErrorCallback is called when the TTS client encounters an error, this usually
	// means the TTS client has been cancelled
	ErrorCallback func(error)

	EncodingInfo audio.EncodingInfo
}

type TextToSpeechOption func(*TextToSpeechOptions)

// WithAudioCallback sets the callback for audio data
//
// Deprecated: (since v0.0.14) use [WithSpeechAudioCallback] instead
func WithAudioCallback(callback func([]byte)) TextToSpeechOption {
	return func(o *TextToSpeechOptions) {
		o.AudioCallback = callback
		o.SpeechAudioCallback = callback
	}
}

func WithSpeechAudioCallback(callback func([]byte)) TextToSpeechOption {
	return func(o *TextToSpeechOptions) {
		o.AudioCallback = callback
		o.SpeechAudioCallback = callback
	}
}

// WithAudioEndedCallback sets the callback for when the TTS client has finished producing audio
//
// Deprecated: (since v0.0.14) use [WithSpeechMarkCallback] instead
func WithAudioEndedCallback(callback func(transcript string)) TextToSpeechOption {
	return func(o *TextToSpeechOptions) {
		o.AudioEnded = callback
		o.SpeechMarkCallback = callback
	}
}

func WithSpeechMarkCallback(callback func(string)) TextToSpeechOption {
	return func(o *TextToSpeechOptions) {
		o.SpeechMarkCallback = callback
		o.AudioEnded = callback
	}
}

// WithSpeechEndedCallbackV0 sets the callback for when the TTS client has
// finished producing all required speech
//
// Not supported by all TTS clients
func WithSpeechEndedCallbackV0(callback func(SpeechEndedReport)) TextToSpeechOption {
	return func(o *TextToSpeechOptions) { o.SpeechEndedCallbackV0 = callback }
}

func WithErrorCallback(callback func(error)) TextToSpeechOption {
	return func(o *TextToSpeechOptions) { o.ErrorCallback = callback }
}

func WithEncodingInfo(encodingInfo audio.EncodingInfo) TextToSpeechOption {
	return func(o *TextToSpeechOptions) {
		if encodingInfo.SampleRate == 0 || encodingInfo.Encoding == "" {
			// TODO: Issue warning
			return
		}

		o.EncodingInfo = encodingInfo
	}
}

type SpeechGeneratorV0 interface {
	// SendText sends text to [SpeechGenerator]. It is guaranteed that the
	// speech will be generated in the order text is sent.
	//
	// SendText will error if EndOfText, Cancel or Close has been called.
	SendText(string) error
	// Mark marks the current point in the text. It is guaranteed that the mark
	// will be returned after the text sent up to the mark has been generated.
	// There is no guarantee that the mark will be returned exactly at the point
	// where it was marked.
	//
	// Mark will error if EndOfText, Cancel or Close has been called.
	Mark() error
	// EndOfText sends a signal to the [SpeechGenerator]that no more text will
	// be sent. After EndOfText is called, [SpeechGenerator] will Close after
	// all the speech has been generated.
	//
	// EndOfText will error if Cancel or Close has been called.
	// Repeated calls to EndOfText are ignored.
	EndOfText() error
	// Cancel immediately cancels the further speech generation. It also closes
	// [SpeechGenerator].
	//
	// This will error if Close has been called.
	// Repeated calls to Cancel are ignored.
	Cancel() error
	// Close immediately closes the [SpeechGenerator]. It is guaranteed that no
	// more speech will be generated after this call.
	//
	// Repeated calls to Close are ignored.
	Close() error
}

type SpeechEndedReport struct{}

// TODO: Extend the report with more information
// type SpeechEndedPosition struct {
// 	Text          string
// 	StartPosition int
// 	EndPosition   int
// }
