package elevenlabs

import (
	"testing"

	"github.com/koscakluka/ema-core/core/audio"
)

func TestConvertEncoding(t *testing.T) {
	testCases := []struct {
		name          string
		encoding      audio.EncodingInfo
		expected      string
		expectFailure bool
	}{
		{
			name: "linear16 16k",
			encoding: audio.EncodingInfo{
				SampleRate: 16000,
				Format:     audio.EncodingLinear16,
			},
			expected: "pcm_16000",
		},
		{
			name: "linear16 24k",
			encoding: audio.EncodingInfo{
				SampleRate: 24000,
				Format:     audio.EncodingLinear16,
			},
			expected: "pcm_24000",
		},
		{
			name: "mulaw 8k",
			encoding: audio.EncodingInfo{
				SampleRate: 8000,
				Format:     audio.EncodingMulaw,
			},
			expected: "ulaw_8000",
		},
		{
			name: "alaw invalid rate",
			encoding: audio.EncodingInfo{
				SampleRate: 16000,
				Format:     audio.EncodingALaw,
			},
			expectFailure: true,
		},
		{
			name: "linear16 invalid rate",
			encoding: audio.EncodingInfo{
				SampleRate: 48000,
				Format:     audio.EncodingLinear16,
			},
			expectFailure: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			converted, err := convertEncoding(testCase.encoding)
			if testCase.expectFailure {
				if err == nil {
					t.Fatalf("expected conversion to fail")
				}
				return
			}

			if err != nil {
				t.Fatalf("expected conversion to pass, got error: %v", err)
			}
			if converted.OutputFormat != testCase.expected {
				t.Fatalf("expected output format %q, got %q", testCase.expected, converted.OutputFormat)
			}
		})
	}
}
