package soniox

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/koscakluka/ema-core/core/audio"
)

func (s *TranscriptionClient) resolveAPIKey() (string, error) {
	if s.apiKey != "" {
		return s.apiKey, nil
	}

	if apiKey := os.Getenv(envVarAPIKeyName); apiKey != "" {
		return apiKey, nil
	}

	return "", fmt.Errorf("soniox api key neither found (%s) nor provided", envVarAPIKeyName)
}

func isExpectedShutdownError(err error) bool {
	if err == nil {
		return true
	}

	if errors.Is(err, net.ErrClosed) {
		return true
	}

	if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		return true
	}

	return strings.Contains(err.Error(), "use of closed network connection")
}

func newSilenceChunk(encodingInfo audio.EncodingInfo, duration time.Duration) ([]byte, error) {
	if encodingInfo.SampleRate <= 0 {
		return nil, fmt.Errorf("invalid sample rate")
	}

	byteSize := encodingInfo.Format.ByteSize()
	if byteSize <= 0 {
		return nil, fmt.Errorf("unsupported encoding format")
	}

	frameCount := int((time.Duration(encodingInfo.SampleRate) * duration) / time.Second)
	if frameCount <= 0 {
		return nil, fmt.Errorf("invalid silence frame duration")
	}

	chunk := make([]byte, frameCount*byteSize)
	for i := range chunk {
		chunk[i] = encodingInfo.SilenceValue()
	}

	return chunk, nil
}
