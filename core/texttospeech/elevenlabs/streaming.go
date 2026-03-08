package elevenlabs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/texttospeech"
)

type streamingRequest struct {
	ws *websocket.Conn

	writeMu sync.Mutex
	mu      sync.Mutex

	options texttospeech.TextToSpeechOptions

	marker marker

	textComplete bool
	cancelled    bool
	closed       bool
	ended        bool

	report texttospeech.SpeechEndedReport
}

func (c *TextToSpeechClient) NewSpeechGeneratorV0(ctx context.Context, opts ...texttospeech.TextToSpeechOption) (texttospeech.SpeechGeneratorV0, error) {
	if c == nil {
		return nil, fmt.Errorf("elevenlabs tts client is nil")
	}

	apiKey, err := c.resolveAPIKey()
	if err != nil {
		return nil, err
	}

	request := &streamingRequest{
		options: texttospeech.TextToSpeechOptions{
			AudioCallback:         func([]byte) {},
			AudioEnded:            func(string) {},
			SpeechAudioCallback:   func([]byte) {},
			SpeechMarkCallback:    func(string) {},
			SpeechEndedCallbackV0: func(texttospeech.SpeechEndedReport) {},
			ErrorCallback:         func(error) {},
			EncodingInfo:          audio.GetDefaultEncodingInfo(),
		},
	}

	for _, opt := range opts {
		opt(&request.options)
	}

	encoding, err := convertEncoding(request.options.EncodingInfo)
	if err != nil {
		return nil, fmt.Errorf("invalid encoding: %w", err)
	}

	conn, err := c.connectWebsocket(ctx, encoding, apiKey)
	if err != nil {
		return nil, err
	}

	request.ws = conn

	if err := request.sendWebsocketMessage(initialiseConnectionMessage{
		Text:             " ",
		VoiceSettings:    c.voiceSettings,
		GenerationConfig: c.generationConfig,
	}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to initialize elevenlabs stream: %w", err)
	}

	go request.processIncomingMessages()
	go func() {
		<-ctx.Done()
		_ = request.Close()
	}()

	return request, nil
}

type alignment struct {
	Chars            []string `json:"chars"`
	CharStartTimesMs []int    `json:"charStartTimesMs"`
	CharDurationsMs  []int    `json:"charDurationsMs"`
}

type receiveMessage struct {
	Audio               string     `json:"audio"`
	Alignment           *alignment `json:"alignment"`
	NormalizedAlignment *alignment `json:"normalizedAlignment"`
	IsFinal             bool       `json:"isFinal"`
}

type initialiseConnectionMessage struct {
	Text             string                 `json:"text"`
	VoiceSettings    *RealtimeVoiceSettings `json:"voice_settings,omitempty"`
	GenerationConfig *GenerationConfig      `json:"generation_config,omitempty"`
	// TODO: Add pronunciation_dictionary_locators support.
}

type sendTextMessage struct {
	Text                 string `json:"text"`
	Flush                bool   `json:"flush,omitempty"`
	TryTriggerGeneration bool   `json:"try_trigger_generation,omitempty"`
}

type closeConnectionMessage struct {
	Text string `json:"text"`
}

func (r *streamingRequest) SendText(text string) error {
	if text == "" {
		return nil
	}

	if err := r.ensureWritable(); err != nil {
		return err
	}

	if err := r.sendWebsocketMessage(sendTextMessage{Text: text}); err != nil {
		return fmt.Errorf("failed to send text to elevenlabs websocket: %w", err)
	}

	r.marker.AddText(text)
	return nil
}

func (r *streamingRequest) Mark() error {
	if err := r.ensureWritable(); err != nil {
		return err
	}

	r.marker.NewMark()
	return nil
}

func (r *streamingRequest) EndOfText() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return fmt.Errorf("streaming request closed")
	}
	if r.cancelled {
		r.mu.Unlock()
		return fmt.Errorf("streaming request cancelled")
	}
	if r.textComplete {
		r.mu.Unlock()
		return nil
	}
	r.textComplete = true
	r.mu.Unlock()

	r.marker.Finalise()
	if err := r.sendWebsocketMessage(closeConnectionMessage{Text: ""}); err != nil {
		return fmt.Errorf("failed to send close message to elevenlabs websocket: %w", err)
	}

	return nil
}

func (r *streamingRequest) Cancel() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return fmt.Errorf("streaming request closed")
	}
	if r.cancelled {
		r.mu.Unlock()
		return nil
	}
	r.cancelled = true
	r.mu.Unlock()

	if err := r.sendWebsocketMessage(closeConnectionMessage{Text: ""}); err != nil {
		if !isExpectedShutdownError(err) {
			r.options.ErrorCallback(fmt.Errorf("failed to cancel elevenlabs stream: %w", err))
		}
	}

	if err := r.closeSocket(); err != nil {
		return err
	}

	return nil
}

func (r *streamingRequest) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	var closeSignalErr error
	if err := r.sendWebsocketMessage(closeConnectionMessage{Text: ""}); err != nil && !isExpectedShutdownError(err) {
		closeSignalErr = fmt.Errorf("failed to send close message to elevenlabs websocket: %w", err)
	}

	closeSocketErr := r.closeSocket()
	if closeSignalErr != nil || closeSocketErr != nil {
		return errors.Join(closeSignalErr, closeSocketErr)
	}

	return nil
}

func (r *streamingRequest) ensureWritable() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return fmt.Errorf("streaming request closed")
	}
	if r.cancelled {
		return fmt.Errorf("streaming request cancelled")
	}
	if r.textComplete {
		return fmt.Errorf("streaming request text already completed")
	}

	return nil
}

func (r *streamingRequest) processIncomingMessages() {
	for {
		msgType, msg, err := r.ws.ReadMessage()
		if err != nil {
			if !isExpectedShutdownError(err) {
				r.options.ErrorCallback(fmt.Errorf("elevenlabs websocket read failed: %w", err))
			}
			_ = r.closeSocket()
			return
		}

		if msgType != websocket.TextMessage {
			continue
		}

		var payload receiveMessage
		if err := json.Unmarshal(msg, &payload); err != nil {
			r.options.ErrorCallback(fmt.Errorf("failed to decode elevenlabs websocket message: %w", err))
			continue
		}
		if payload.Audio != "" {
			if payload.Alignment == nil {
				payload.Alignment = payload.NormalizedAlignment
			}

			for element, playErr := range r.marker.Play(payload, r.options.EncodingInfo) {
				if playErr != nil {
					r.options.ErrorCallback(fmt.Errorf("failed to process elevenlabs marker payload: %w", playErr))
					break
				}

				switch element.Type {
				case markerElementTypeAudio:
					if len(element.Audio) > 0 {
						r.options.SpeechAudioCallback(element.Audio)
					}

				case markerElementTypeMark:
					r.options.SpeechMarkCallback(element.Mark)
				}
			}
		}

		if payload.IsFinal {
			r.finishStream()
			return
		}
	}
}

func (r *streamingRequest) finishStream() {
	r.mu.Lock()
	if r.ended {
		r.mu.Unlock()
		return
	}
	r.ended = true
	r.mu.Unlock()

	pendingMarks := r.marker.FlushPendingMarks()
	for _, mark := range pendingMarks {
		r.options.SpeechMarkCallback(mark)
	}

	r.options.SpeechEndedCallbackV0(r.report)

	if err := r.closeSocket(); err != nil {
		log.Printf("failed to close elevenlabs socket after final message: %v", err)
	}
}

func (r *streamingRequest) sendWebsocketMessage(msg any) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	r.mu.Lock()
	closed := r.closed
	ws := r.ws
	r.mu.Unlock()

	if closed || ws == nil {
		return fmt.Errorf("websocket connection closed")
	}

	if err := ws.WriteJSON(msg); err != nil {
		return fmt.Errorf("failed to write to websocket: %w", err)
	}

	return nil
}

func (r *streamingRequest) closeSocket() error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	r.mu.Lock()
	ws := r.ws
	r.ws = nil
	r.closed = true
	r.mu.Unlock()

	if ws == nil {
		return nil
	}

	if err := ws.Close(); err != nil && !isExpectedShutdownError(err) {
		return fmt.Errorf("failed to close websocket: %w", err)
	}

	return nil
}

func isExpectedShutdownError(err error) bool {
	if err == nil {
		return false
	}

	if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		return true
	}

	errText := err.Error()
	return strings.Contains(errText, "use of closed network connection")
}
