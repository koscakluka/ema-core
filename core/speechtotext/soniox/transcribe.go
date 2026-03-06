package soniox

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/speechtotext"
)

const (
	websocketURL = "wss://stt-rt.soniox.com/transcribe-websocket"
	// Soniox supports endpoint delay in [500, 3000]ms.
	defaultMaxEndpointDelayMs = 1000
	// Soniox expects keepalive when no audio is sent for ~20s.
	// The default is intentionally lower to provide margin.
	defaultKeepalive  = 10 * time.Second
	keepaliveTickRate = 5 * time.Second

	silenceFrameDuration = 50 * time.Millisecond
	audioIdleThreshold   = 100 * time.Millisecond
)

type tokenType string

const (
	tokenTypeEndpoint tokenType = "<end>"
	tokenTypeFinalize tokenType = "<fin>"
)

// controlMessage carries Soniox real-time control operations.

// responseToken mirrors token fields used by EMA callback mapping.
type responseToken struct {
	Text    string `json:"text"`
	IsFinal bool   `json:"is_final"`
}

// responseMessage mirrors Soniox realtime response fields used by this client.
type responseMessage struct {
	Tokens       []responseToken `json:"tokens"`
	Finished     bool            `json:"finished"`
	ErrorCode    int             `json:"error_code"`
	ErrorMessage string          `json:"error_message"`
}

// transcriptionState tracks currently active utterance assembly.
//
// - finalTranscript accumulates finalized tokens only.
// - TODO: Add interim/final language tracking once callback API is enabled.
type transcriptionState struct {
	inSpeech        bool
	finalTranscript strings.Builder
}

// Transcribe starts a Soniox session and attaches callback mapping logic.
//
// The method sends a Soniox start request once, then starts background
// goroutines for keepalive and inbound response processing.
func (s *TranscriptionClient) Transcribe(ctx context.Context, opts ...speechtotext.TranscriptionOption) error {
	options := &speechtotext.TranscriptionOptions{EncodingInfo: audio.GetDefaultEncodingInfo()}
	for _, opt := range opts {
		opt(options)
	}

	callbacks := newCallbackConfig(*options)

	encoding, err := convertEncoding(options.EncodingInfo)
	if err != nil {
		return fmt.Errorf("invalid encoding: %w", err)
	}

	apiKey, err := s.resolveAPIKey()
	if err != nil {
		return err
	}

	conn, err := s.openWebsocket(ctx)
	if err != nil {
		return err
	}

	type startRequest struct {
		APIKey                  string `json:"api_key"`
		Model                   Model  `json:"model"`
		AudioFormat             string `json:"audio_format"`
		SampleRate              int    `json:"sample_rate"`
		NumChannels             int    `json:"num_channels"`
		EnableEndpointDetection bool   `json:"enable_endpoint_detection"`
		MaxEndpointDelayMs      int    `json:"max_endpoint_delay_ms,omitempty"`
	}

	if err := conn.WriteJSON(startRequest{
		APIKey:                  apiKey,
		Model:                   defaultModel,
		AudioFormat:             string(encoding.AudioFormat),
		SampleRate:              encoding.SampleRate,
		NumChannels:             encoding.NumChannels,
		EnableEndpointDetection: true,
		MaxEndpointDelayMs:      defaultMaxEndpointDelayMs,
	}); err != nil {
		_ = conn.Close()
		return fmt.Errorf("failed to send soniox start request: %w", err)
	}

	s.connMu.Lock()
	s.conn = conn
	s.connMu.Unlock()

	keepaliveCtx, keepaliveCancel := context.WithCancel(ctx)
	if s.keepaliveCancel != nil {
		s.keepaliveCancel()
	}
	s.keepaliveCancel = keepaliveCancel
	s.state = transcriptionState{}

	now := time.Now().UnixNano()
	atomic.StoreInt64(&s.lastAudioReceivedAt, now)

	go s.manageConnectionActivity(keepaliveCtx, options.EncodingInfo)
	go s.readAndProcessMessages(conn, callbacks)

	return nil
}

func (s *TranscriptionClient) openWebsocket(ctx context.Context) (*websocket.Conn, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, websocketURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open socket connection to soniox: %w", err)
	}

	return conn, nil
}

// SendAudio streams a single raw audio chunk to Soniox.
//
// Audio must match the encoding provided when Transcribe was started.
func (s *TranscriptionClient) SendAudio(audio []byte) error {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	if s.conn == nil {
		return fmt.Errorf("soniox websocket is not connected")
	}

	atomic.StoreInt64(&s.lastAudioReceivedAt, time.Now().UnixNano())
	if err := s.conn.WriteMessage(websocket.BinaryMessage, audio); err != nil {
		return fmt.Errorf("failed to write to soniox websocket: %w", err)
	}

	return nil
}

func (s *TranscriptionClient) sendSilence(audio []byte) error {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	if s.conn == nil {
		return nil
	}

	if err := s.conn.WriteMessage(websocket.BinaryMessage, audio); err != nil {
		return fmt.Errorf("failed to write silence to soniox websocket: %w", err)
	}

	return nil
}

// sendKeepalive sends one Soniox keepalive control message.
func (s *TranscriptionClient) sendKeepalive() error {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	if s.conn == nil {
		return nil
	}

	if err := s.conn.WriteJSON(struct {
		Type string `json:"type"`
	}{Type: "keepalive"}); err != nil {
		return fmt.Errorf("failed to write keepalive to soniox websocket: %w", err)
	}

	return nil
}

// readAndProcessMessages reads Soniox JSON responses and dispatches them to
// callback mapping.
func (s *TranscriptionClient) readAndProcessMessages(conn *websocket.Conn, callbacks callbackConfig) {
	defer func() {
		s.connMu.Lock()
		if s.keepaliveCancel != nil {
			s.keepaliveCancel()
			s.keepaliveCancel = nil
		}
		if s.conn == conn {
			s.conn = nil
		}
		s.connMu.Unlock()
		_ = conn.Close()
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if !isExpectedShutdownError(err) {
				log.Println("Failed to read soniox websocket message", "error", err)
			}
			return
		}

		s.processMessage(msg, callbacks)
	}
}

func (s *TranscriptionClient) manageConnectionActivity(ctx context.Context, encodingInfo audio.EncodingInfo) {
	type state string
	const (
		stateWaiting   state = "waiting"
		stateSilence   state = "silence"
		stateKeepalive state = "keepalive"
	)

	silenceWindow := time.Duration(defaultMaxEndpointDelayMs) * time.Millisecond
	if silenceWindow < 0 {
		silenceWindow = 0
	}

	silenceChunk, err := newSilenceChunk(encodingInfo, silenceFrameDuration)
	if err != nil {
		log.Println("Failed to build soniox silence chunk", "error", err)
		silenceWindow = 0
	}

	ticker := time.NewTicker(silenceFrameDuration)
	defer ticker.Stop()

	currentState := stateWaiting
	var firstSilenceAt *time.Time
	var lastKeepaliveAt *time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			lastAudioReceivedAt := atomic.LoadInt64(&s.lastAudioReceivedAt)
			sinceLastAudio := time.Since(time.Unix(0, lastAudioReceivedAt))

			if sinceLastAudio <= audioIdleThreshold {
				currentState = stateWaiting
				firstSilenceAt = nil
				lastKeepaliveAt = nil
				continue
			}

			now := time.Now()

			switch currentState {
			case stateWaiting:
				if silenceWindow > 0 {
					firstSilenceAt = &now
					currentState = stateSilence
				} else {
					currentState = stateKeepalive
				}

			case stateSilence:
				if firstSilenceAt == nil {
					firstSilenceAt = &now
				}

				if time.Since(*firstSilenceAt) >= silenceWindow {
					firstSilenceAt = nil
					currentState = stateKeepalive
					continue
				}

				if err := s.sendSilence(silenceChunk); err != nil && !isExpectedShutdownError(err) {
					log.Println("Sending soniox silence audio error", "error", err)
				}

			case stateKeepalive:
				if sinceLastAudio < defaultKeepalive {
					continue
				}

				if lastKeepaliveAt != nil && time.Since(*lastKeepaliveAt) < keepaliveTickRate {
					continue
				}

				if err := s.sendKeepalive(); err != nil {
					if !isExpectedShutdownError(err) {
						log.Println("Failed to send soniox keepalive", "error", err)
					}
					continue
				}

				keepaliveSentAt := time.Now()
				lastKeepaliveAt = &keepaliveSentAt
			}
		}
	}
}

// processMessage handles one raw Soniox text message.
func (s *TranscriptionClient) processMessage(msg []byte, callbacks callbackConfig) {
	var parsedMsg responseMessage
	if err := json.Unmarshal(msg, &parsedMsg); err != nil {
		log.Println("Failed to unmarshal soniox message", "error", err)
		return
	}

	if parsedMsg.ErrorCode != 0 {
		log.Printf("Soniox response error %d: %s", parsedMsg.ErrorCode, parsedMsg.ErrorMessage)
		return
	}

	s.processResponse(parsedMsg, callbacks)
}

// processResponse maps Soniox token streams to EMA callback semantics.
//
// Mapping summary:
//   - non-final tokens -> partial+full interim transcription callbacks
//   - final tokens -> partial finalized transcription callback
//   - endpoint/finalize markers (<end>/<fin>) or finished=true -> speech end
//   - TODO: Re-enable language callback mapping once API is finalized
func (s *TranscriptionClient) processResponse(msg responseMessage, callbacks callbackConfig) {
	var interimTextBuilder strings.Builder
	var finalTextBuilder strings.Builder

	markerFinalized := false
	speechObserved := false
	hasInterimToken := false

	// Soniox may send both final and non-final tokens in the same response.
	// We split them first, then dispatch callbacks in stable order.
	for _, token := range msg.Tokens {
		if token.Text == "" {
			continue
		}

		if isFinalizationToken(token) {
			markerFinalized = true
			continue
		}

		if strings.TrimSpace(token.Text) != "" {
			speechObserved = true
		}

		if token.IsFinal {
			finalTextBuilder.WriteString(token.Text)
			continue
		}

		interimTextBuilder.WriteString(token.Text)
		hasInterimToken = true
	}

	if speechObserved && !s.state.inSpeech {
		s.state.inSpeech = true
		callbacks.startSpeechCallback()
	}

	finalChunk := finalTextBuilder.String()
	if finalChunk != "" {
		s.state.finalTranscript.WriteString(finalChunk)
		callbacks.partialTranscriptionCallback(finalChunk)
	}

	interimChunk := interimTextBuilder.String()
	if hasInterimToken {
		callbacks.partialInterimTranscriptionCallback(interimChunk)
		callbacks.interimTranscriptionCallback(s.state.finalTranscript.String() + interimChunk)
	}

	if markerFinalized || msg.Finished {
		s.onSpeechEnded(callbacks)
	}
}

// onSpeechEnded emits final transcript callbacks and resets per-
// utterance state.
func (s *TranscriptionClient) onSpeechEnded(callbacks callbackConfig) {
	if !s.state.inSpeech {
		return
	}

	s.state.inSpeech = false

	fullTranscript := strings.TrimSpace(s.state.finalTranscript.String())
	if fullTranscript != "" {
		callbacks.transcriptionCallback(fullTranscript)
	}

	callbacks.endSpeechCallback()
	s.state = transcriptionState{}
}

// isFinalizationToken checks for Soniox finalization markers.
func isFinalizationToken(token responseToken) bool {
	if !token.IsFinal {
		return false
	}

	switch tokenType(token.Text) {
	case tokenTypeEndpoint, tokenTypeFinalize:
		return true
	default:
		return false
	}
}

// callbackConfig stores normalized callbacks where nil handlers are replaced
// with no-ops.
type callbackConfig struct {
	partialInterimTranscriptionCallback func(string)
	interimTranscriptionCallback        func(string)
	partialTranscriptionCallback        func(string)
	transcriptionCallback               func(string)
	startSpeechCallback                 func()
	endSpeechCallback                   func()
}

// newCallbackConfig converts TranscriptionOptions into guaranteed non-nil
// callback handlers.
func newCallbackConfig(options speechtotext.TranscriptionOptions) callbackConfig {
	callbacks := callbackConfig{
		partialInterimTranscriptionCallback: options.PartialInterimTranscriptionCallback,
		interimTranscriptionCallback:        options.InterimTranscriptionCallback,
		partialTranscriptionCallback:        options.PartialTranscriptionCallback,
		transcriptionCallback:               options.TranscriptionCallback,
		startSpeechCallback:                 options.SpeechStartedCallback,
		endSpeechCallback:                   options.SpeechEndedCallback,
	}

	if callbacks.partialInterimTranscriptionCallback == nil {
		callbacks.partialInterimTranscriptionCallback = func(string) {}
	}
	if callbacks.interimTranscriptionCallback == nil {
		callbacks.interimTranscriptionCallback = func(string) {}
	}
	if callbacks.partialTranscriptionCallback == nil {
		callbacks.partialTranscriptionCallback = func(string) {}
	}
	if callbacks.transcriptionCallback == nil {
		callbacks.transcriptionCallback = func(string) {}
	}
	if callbacks.startSpeechCallback == nil {
		callbacks.startSpeechCallback = func() {}
	}
	if callbacks.endSpeechCallback == nil {
		callbacks.endSpeechCallback = func() {}
	}

	return callbacks
}
