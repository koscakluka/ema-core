package audio

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
