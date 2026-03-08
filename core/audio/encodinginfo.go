package audio

import "time"

const (
	DefaultSampleRate = 16000
	DefaultFormat     = "linear16"
)

func GetDefaultEncodingInfo() EncodingInfo {
	return EncodingInfo{SampleRate: DefaultSampleRate, Format: encodingFormat(DefaultFormat)}
}

type EncodingInfo struct {
	SampleRate int
	Format     encodingFormat
}

func (e EncodingInfo) IsZero() bool {
	return e.SampleRate == 0 || e.Format.Name() == ""
}

func (e EncodingInfo) SilenceValue() byte {
	switch e.Format {
	case encodingFormat("alaw"):
		return 0x55
	case encodingFormat("mulaw"):
		return 0xFF
	case encodingFormat("linear16"):
		return 0
	}

	return 0
}

func (e EncodingInfo) BytesPerSecond() int {
	if e.SampleRate <= 0 {
		return 0
	}

	byteSize := e.Format.ByteSize()
	if byteSize <= 0 {
		return 0
	}

	return e.SampleRate * byteSize
}

func (e EncodingInfo) BytesForDuration(duration time.Duration) int {
	return int(float64(duration) / float64(time.Second) * float64(e.BytesPerSecond()))
}

func (e EncodingInfo) DurationForBytes(bytes int) time.Duration {
	if bytes <= 0 {
		return 0
	}

	bytesPerSecond := e.BytesPerSecond()
	if bytesPerSecond <= 0 {
		return 0
	}

	return time.Duration(float64(bytes) / float64(bytesPerSecond) * float64(time.Second))
}

type encodingFormat string

func (e encodingFormat) Name() string {
	return string(e)
}

func (e encodingFormat) ByteSize() int {
	switch e {
	case encodingFormat("mulaw"), encodingFormat("alaw"):
		return 1
	case encodingFormat("linear16"):
		return 2
	}
	return -1
}

const (
	EncodingMulaw    encodingFormat = "mulaw"
	EncodingALaw     encodingFormat = "alaw"
	EncodingLinear16 encodingFormat = "linear16"
)
