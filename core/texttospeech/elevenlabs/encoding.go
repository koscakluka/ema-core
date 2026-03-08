package elevenlabs

import (
	"fmt"

	"github.com/koscakluka/ema-core/core/audio"
)

type encodingInfo struct {
	OutputFormat string
}

func convertEncoding(encoding audio.EncodingInfo) (encodingInfo, error) {
	switch encoding.Format {
	case audio.EncodingLinear16:
		switch encoding.SampleRate {
		case 8000:
			return encodingInfo{OutputFormat: "pcm_8000"}, nil
		case 16000:
			return encodingInfo{OutputFormat: "pcm_16000"}, nil
		case 22050:
			return encodingInfo{OutputFormat: "pcm_22050"}, nil
		case 24000:
			return encodingInfo{OutputFormat: "pcm_24000"}, nil
		case 44100:
			return encodingInfo{OutputFormat: "pcm_44100"}, nil
		default:
			return encodingInfo{}, fmt.Errorf("unsupported sample rate for linear16: %d", encoding.SampleRate)
		}
	case audio.EncodingALaw:
		if encoding.SampleRate != 8000 {
			return encodingInfo{}, fmt.Errorf("unsupported sample rate for alaw: %d", encoding.SampleRate)
		}
		return encodingInfo{OutputFormat: "alaw_8000"}, nil
	case audio.EncodingMulaw:
		if encoding.SampleRate != 8000 {
			return encodingInfo{}, fmt.Errorf("unsupported sample rate for mulaw: %d", encoding.SampleRate)
		}
		return encodingInfo{OutputFormat: "ulaw_8000"}, nil
	default:
		return encodingInfo{}, fmt.Errorf("unsupported encoding format: %q", encoding.Format.Name())
	}
}
