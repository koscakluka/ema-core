package soniox

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const envVarAPIKeyName = "SONIOX_API_KEY"

// TranscriptionClient streams audio to Soniox and maps responses to EMA STT callbacks.
type TranscriptionClient struct {
	apiKey string

	conn            *websocket.Conn
	connMu          sync.Mutex
	keepaliveCancel context.CancelFunc

	lastAudioReceivedAt int64

	state transcriptionState
}

// NewClient creates a Soniox transcription client.
//
// Defaults:
//   - Endpoint detection: enabled
//   - Max endpoint delay: 1000ms
//
// The context parameter is currently unused and kept for constructor parity.
func NewClient(_ context.Context, opts ...ClientOption) *TranscriptionClient {
	options := ClientOptions{}

	for _, opt := range opts {
		opt(&options)
	}

	return &TranscriptionClient{
		apiKey: options.APIKey,
	}
}

// Close gracefully closes the active Soniox stream and socket.
//
// It first sends an empty binary frame to indicate end-of-audio, then sends a
// normal close control frame and closes the WebSocket transport.
func (s *TranscriptionClient) Close() error {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	if s.keepaliveCancel != nil {
		s.keepaliveCancel()
		s.keepaliveCancel = nil
	}

	if s.conn == nil {
		return nil
	}

	err := s.conn.WriteMessage(websocket.BinaryMessage, []byte{})
	if isExpectedShutdownError(err) {
		err = nil
	}

	closeControlErr := s.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second),
	)
	if isExpectedShutdownError(closeControlErr) {
		closeControlErr = nil
	}

	closeErr := s.conn.Close()
	if isExpectedShutdownError(closeErr) {
		closeErr = nil
	}
	s.conn = nil

	if err != nil && closeControlErr != nil && closeErr != nil {
		return fmt.Errorf("failed to close soniox websocket: %w", errors.Join(err, closeControlErr, closeErr))
	}
	if err != nil && closeControlErr != nil {
		return fmt.Errorf("failed to close soniox websocket: %w", errors.Join(err, closeControlErr))
	}
	if closeControlErr != nil && closeErr != nil {
		return fmt.Errorf("failed to close soniox websocket: %w", errors.Join(closeControlErr, closeErr))
	}
	if err != nil {
		return fmt.Errorf("failed to signal soniox stream end: %w", err)
	}
	if closeControlErr != nil {
		return fmt.Errorf("failed to send soniox close control frame: %w", closeControlErr)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close soniox websocket: %w", closeErr)
	}

	return nil
}

type ClientOption func(*ClientOptions)

// ClientOptions controls Soniox session behavior.
//
// Each field maps directly to Soniox real-time start-request parameters.
type ClientOptions struct {
	// APIKey overrides SONIOX_API_KEY for this client.
	APIKey string
	// TODO: Re-introduce language-identification configuration once public
	// language callback APIs are stable.
}

// WithAPIKey sets the API key explicitly.
//
// If omitted, SONIOX_API_KEY is used.
func WithAPIKey(apiKey string) ClientOption {
	return func(o *ClientOptions) { o.APIKey = apiKey }
}
