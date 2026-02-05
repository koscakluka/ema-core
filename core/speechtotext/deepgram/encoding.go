package deepgram

import (
	"fmt"

	"github.com/koscakluka/ema-core/core/audio"
)

type encodingInfo struct {
	SampleRate int
	Format     encodingFormat
}

type encodingFormat string

func (e encodingFormat) Name() string { return string(e) }

const (
	encodingLinear16 encodingFormat = "linear16"
	encodingALaw     encodingFormat = "alaw"
	encodingMulaw    encodingFormat = "mulaw"
)

func convertEncoding(encoding audio.EncodingInfo) (*encodingInfo, error) {
	deepgramEncoding := encodingInfo{}
	switch encoding.SampleRate {
	case 8000, 16000, 24000, 32000, 48000:
		deepgramEncoding.SampleRate = encoding.SampleRate
	default:
		return nil, fmt.Errorf("unsupported sample rate")
	}

	// TODO: Ensure that correct sample rate is used where they are limited
	switch encoding.Format {
	case audio.EncodingLinear16:
		deepgramEncoding.Format = encodingLinear16
	case audio.EncodingALaw:
		deepgramEncoding.Format = encodingALaw
		if deepgramEncoding.SampleRate != 8000 {
			return nil, fmt.Errorf("unsupported sample rate for alaw encoding")
		}
	case audio.EncodingMulaw:
		deepgramEncoding.Format = encodingMulaw
		if deepgramEncoding.SampleRate != 8000 {
			return nil, fmt.Errorf("unsupported sample rate for alaw encoding")
		}
	default:
		return nil, fmt.Errorf("unsupported encoding")
	}

	return &deepgramEncoding, nil
}
