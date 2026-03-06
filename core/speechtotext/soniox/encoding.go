package soniox

import (
	"fmt"

	"github.com/koscakluka/ema-core/core/audio"
)

type encodingFormat string

const (
	audioFormatPCMS16LE encodingFormat = "pcm_s16le"
	audioFormatALaw     encodingFormat = "alaw"
	audioFormatMulaw    encodingFormat = "mulaw"
)

type encodingInfo struct {
	AudioFormat encodingFormat
	SampleRate  int
	NumChannels int
}

// convertEncoding maps EMA audio encoding to Soniox raw audio parameters.
//
// TODO: EMA currently captures mono audio, so NumChannels is fixed to 1, we
// should allow passing multi channel encoding.
func convertEncoding(encoding audio.EncodingInfo) (*encodingInfo, error) {
	if encoding.SampleRate <= 0 {
		return nil, fmt.Errorf("unsupported sample rate")
	}

	sonioxEncoding := &encodingInfo{
		SampleRate:  encoding.SampleRate,
		NumChannels: 1,
	}

	switch encoding.Format {
	case audio.EncodingLinear16:
		sonioxEncoding.AudioFormat = audioFormatPCMS16LE
	case audio.EncodingALaw:
		sonioxEncoding.AudioFormat = audioFormatALaw
	case audio.EncodingMulaw:
		sonioxEncoding.AudioFormat = audioFormatMulaw
	// TODO: Support more encoding formats, full list:
	// PCM (signed): pcm_s8, pcm_s16, pcm_s24, pcm_s32 (le/be).
	// PCM (unsigned): pcm_u8, pcm_u16, pcm_u24, pcm_u32 (le/be).
	// Float PCM: pcm_f32, pcm_f64 (le/be).
	// Companded: mulaw, alaw.
	default:
		return nil, fmt.Errorf("unsupported encoding")
	}

	return sonioxEncoding, nil
}
