package deepgram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/texttospeech"
)

type streamingRequest struct {
	ws *websocket.Conn
	mu sync.Mutex

	textBuffer   []string
	textBufferMu sync.Mutex

	options streamingRequestOptions

	textComplete bool
	cancelled    bool
	closed       bool

	report texttospeech.SpeechEndedReport
}

type streamingRequestOptions struct {
	texttospeech.TextToSpeechOptions
	Voice deepgramVoice
}

func (c *TextToSpeechClient) NewSpeechGeneratorV0(ctx context.Context, opts ...texttospeech.TextToSpeechOption) (texttospeech.SpeechGeneratorV0, error) {
	req := &streamingRequest{
		options: streamingRequestOptions{
			TextToSpeechOptions: texttospeech.TextToSpeechOptions{
				AudioCallback:         func([]byte) {},
				AudioEnded:            func(string) {},
				SpeechAudioCallback:   func([]byte) {},
				SpeechMarkCallback:    func(string) {},
				SpeechEndedCallbackV0: func(texttospeech.SpeechEndedReport) {},
				ErrorCallback:         func(error) {},
			},
		},
	}

	for _, opt := range opts {
		opt(&req.options.TextToSpeechOptions)
	}

	var err error
	if req.ws, err = connectWebsocket(c.voice, c.options.EncodingInfo); err != nil {
		return nil, fmt.Errorf("failed to open websocket: %w", err)
	}

	go req.processIncomingMessages(ctx)

	return req, nil
}

func connectWebsocket(voice deepgramVoice, encodingInfo audio.EncodingInfo) (*websocket.Conn, error) {
	// TODO: Allow passing API key in constructor
	apiKey, ok := os.LookupEnv("DEEPGRAM_API_KEY")
	if !ok {
		return nil, fmt.Errorf("deepgram api key not found")
	}

	urlValues := url.Values{}
	urlValues.Set("encoding", encodingInfo.Encoding)
	urlValues.Set("sample_rate", strconv.Itoa(encodingInfo.SampleRate))
	urlValues.Set("model", string(voice))
	urlValues.Set("container", "none")

	// TODO: Use DialContext
	conn, _, err := websocket.DefaultDialer.Dial(
		(&url.URL{
			Scheme: "wss",
			Host:   "api.deepgram.com", Path: "/v1/speak",
			RawQuery: urlValues.Encode(),
		}).String(),
		http.Header{"Authorization": {"token " + apiKey}})
	if err != nil {
		return nil, fmt.Errorf("failed to open socket connection to deepgram: %w", err)
	}

	return conn, nil
}

func (r *streamingRequest) processIncomingMessages(ctx context.Context) {

	// TODO: We can probably stop once we close or cancel
	for {
		msgType, msg, err := r.ws.ReadMessage()
		if err != nil {
			// TODO: Actually figure out this message instead of comparing to a string
			if err.Error() != "websocket: close 1000 (normal)" {
				log.Printf("Websocket read error: %v", err)
			}
			if err := r.Cancel(); err != nil {
				_ = r.Close() // Ignored on purpose
				return
			}
			return
		}

		switch msgType {
		case websocket.BinaryMessage:
			r.options.SpeechAudioCallback(msg)
		case websocket.TextMessage:
			var parsedMsg struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(msg, &parsedMsg); err != nil {
				// TODO: Instrument
				// log.Printf("Failed to unmarshal deepgram message: %v", err)
				continue
			}

			switch parsedMsg.Type {
			case "Flushed":
				func() { // Grouped for defer
					r.textBufferMu.Lock()
					defer r.textBufferMu.Unlock()
					// notify the user we have reached the mark
					if len(r.textBuffer) > 0 {
						r.options.SpeechMarkCallback(r.textBuffer[0])
						r.textBuffer = r.textBuffer[1:]
					}

					// nothing left to process, nortify the user of the end
					if len(r.textBuffer) == 0 && r.textComplete {
						r.options.SpeechEndedCallbackV0(r.report)
						_ = r.Close() // TODO: See if we need to react on this error
						return
					}

					// send the next text if there is any
					if len(r.textBuffer) > 0 {
						if err := r.sendWebsocketMessage(sendTextMsg(r.textBuffer[0])); err != nil {
							// TODO: Instrument
							// log.Printf("Failed to speak deepgram text: %v", err)
						}
					}
					// flush if there is even more thxt
					if len(r.textBuffer) > 1 {
						if err := r.sendWebsocketMessage(flushMsg); err != nil {
							// TODO: Instrument
							// log.Printf("Failed to flush deepgram buffer: %v", err)
						}
					}
				}()
			case "Clear":
				// TODO: Handle clear message
			case "Close":
				// TODO: Handle close message
			default:
				// TODO: Handle unknown message types
				// TODO: Instrument
			}
		case websocket.CloseMessage:
			// TODO: Handle close message
		case websocket.PingMessage:
		// TODO: Handle ping message
		case websocket.PongMessage:
			// TODO: Handle pong message
		default:
			// TODO: Handle unknown message types
			// TODO: Instrument
		}
	}
}

func (r *streamingRequest) SendText(text string) error {
	if r.closed {
		return fmt.Errorf("streaming request closed")
	} else if r.cancelled {
		return fmt.Errorf("streaming request cancelled")
	} else if r.textComplete {
		return fmt.Errorf("streaming request text already completed")
	}

	r.textBufferMu.Lock()
	defer r.textBufferMu.Unlock()

	if len(r.textBuffer) == 0 {
		r.textBuffer = append(r.textBuffer, "")
	}

	if len(r.textBuffer) == 1 {
		if err := r.sendWebsocketMessage(sendTextMsg(text)); err != nil {
			return fmt.Errorf("failed to send websocket send text message: %w", err)
		}
	}
	r.textBuffer[len(r.textBuffer)-1] += text
	return nil
}

func (r *streamingRequest) Mark() error {
	if r.closed {
		return fmt.Errorf("streaming request closed")
	} else if r.cancelled {
		return fmt.Errorf("streaming request cancelled")
	} else if r.textComplete {
		return fmt.Errorf("streaming request text already completed")
	}

	r.textBufferMu.Lock()
	defer r.textBufferMu.Unlock()

	if len(r.textBuffer) == 1 {
		if err := r.sendWebsocketMessage(flushMsg); err != nil {
			return fmt.Errorf("failed to send websocket flush message: %w", err)
		}
	}

	// NOTE: For some reason deepgram sometimes drops text that is passed after
	// a flush unless there is some kind of break. This allows us to send the
	// text after we get the flush confirmation
	r.textBuffer = append(r.textBuffer, "")

	return nil
}

func (r *streamingRequest) EndOfText() error {
	if r.closed {
		return fmt.Errorf("streaming request closed")
	} else if r.cancelled {
		return fmt.Errorf("streaming request cancelled")
	}
	r.textBufferMu.Lock()
	defer r.textBufferMu.Unlock()

	r.textComplete = true
	if len(r.textBuffer) == 0 {
		r.options.SpeechEndedCallbackV0(r.report)
		_ = r.Close() // TODO: See if we need to react on this error
	} else if len(r.textBuffer) == 1 && r.textBuffer[0] == "" {
		r.textBuffer = r.textBuffer[1:]
		r.options.SpeechEndedCallbackV0(r.report)
		_ = r.Close() // TODO: See if we need to react on this error
	}

	return nil
}

func (r *streamingRequest) Cancel() error {
	if r.closed {
		return fmt.Errorf("streaming request closed")
	}

	r.cancelled = true
	if err := r.sendWebsocketMessage(clearMsg); err != nil {
		return fmt.Errorf("failed to send websocket close message: %w", err)
	}

	// TODO: This shoud technically be done once we have a confirmation
	_ = r.Close()
	return nil

}

func (r *streamingRequest) Close() error {
	r.closed = true
	if err := r.sendWebsocketMessage(closeMsg); err != nil {
		if agressiveCloseErr := r.ws.Close(); agressiveCloseErr != nil {
			return fmt.Errorf("failed to close websocket: %w", errors.Join(err, agressiveCloseErr))
		}
	}
	return nil
}

type websocketMessage struct {
	Type string `json:"type"`
}

var (
	sendTextMsg = func(text string) struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} { // TODO: This is awkward, it should somehow be the same type
		return struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: "Speak", Text: text}
	}
	flushMsg = websocketMessage{Type: "Flush"}
	clearMsg = websocketMessage{Type: "Clear"}
	closeMsg = websocketMessage{Type: "Close"}
)

func (r *streamingRequest) sendWebsocketMessage(msg any) error {
	// TODO: Find a way to enforce a type instead of having any,
	// specifically [websocketMessage] type
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("websocket connection closed")
	} else if r.ws == nil {
		return fmt.Errorf("websocket connection closed")
	}

	if err := r.ws.WriteJSON(msg); err != nil {
		return fmt.Errorf("failed to write to websocket: %w", err)
	}
	return nil
}

// OpenStream opens a streaming connection to the TTS server
//
// Deprecated: (since v0.0.14) use [TextToSpeechClient].NewSpeechGeneratorV0 instead
func (c *TextToSpeechClient) OpenStream(ctx context.Context, opts ...texttospeech.TextToSpeechOption) error {
	for _, opt := range opts {
		opt(&c.options)
	}

	conn, err := connectWebsocket(c.voice, c.options.EncodingInfo)
	if err != nil {
		return fmt.Errorf("failed to open websocket: %w", err)
	}

	c.wsConn = conn

	go c.readAndProcessMessages(ctx, conn, c.options)

	return nil
}

// SendText sends text to the TTS server
//
// Deprecated: (since v0.0.14) use [texttospeech.SpeechGeneratorV0].SendText instead
func (c *TextToSpeechClient) SendText(text string) error {
	targetBuffer := &c.transcriptBuffer
	if c.postRestartBuffer != nil {
		targetBuffer = &c.postRestartBuffer
	}

	if len(*targetBuffer) == 0 {
		*targetBuffer = append(*targetBuffer, "")
	}

	// TODO: Instead of (or in addition to) a mutex, we could implement a buffer
	// to prevent writing to the websocket at the same time
	if len(*targetBuffer) == 1 {
		if err := c.speak(text); err != nil {
			return err
		}
	}

	(*targetBuffer)[len(*targetBuffer)-1] += text
	return nil
}

func (c *TextToSpeechClient) speak(text string) error {
	if c.wsConn == nil {
		return fmt.Errorf("connection closed")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.wsConn.WriteJSON(struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{
		Type: "Speak",
		Text: text,
	}); err != nil {
		return fmt.Errorf("failed to send text to deepgram through websocket: %w", err)
	}

	return nil
}

// FlushBuffer flushes the TTS buffer
//
// Deprecated: (since v0.0.14) use [texttospeech.SpeechGeneratorV0].Mark instead
func (c *TextToSpeechClient) FlushBuffer() error {
	if len(c.transcriptBuffer) == 1 {
		if err := c.flush(); err != nil {
			return err
		}
	}
	// HACK: For some reason deepgram drops text that is passed after a flush
	// unless there is some kind of break. This allows us to send the text
	// after we get the flush confirmation
	c.transcriptBuffer = append(c.transcriptBuffer, "")

	return nil
}

func (c *TextToSpeechClient) flush() error {
	if c.wsConn == nil {
		return fmt.Errorf("connection closed")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.wsConn.WriteJSON(struct {
		Type string `json:"type"`
	}{
		Type: "Flush",
	}); err != nil {
		return fmt.Errorf("failed to flush deepgram buffer through websocket: %w", err)
	}
	return nil
}

// ClearBuffer clears the TTS buffer
//
// Deprecated: (since v0.0.14) use [texttospeech.SpeechGeneratorV0].Cancel instead
func (c *TextToSpeechClient) ClearBuffer() error {
	if c.wsConn == nil {
		return fmt.Errorf("connection closed")
	}
	if err := c.wsConn.WriteJSON(struct {
		Type string `json:"type"`
	}{
		Type: "Clear",
	}); err != nil {
		return fmt.Errorf("failed to clear deepgram buffer through websocket: %w", err)
	}
	c.transcriptBuffer = []string{}

	return nil
}

// CloseStream closes the streaming connection to the TTS server
//
// Deprecated: (since v0.0.14) use [texttospeech.SpeechGeneratorV0].Close instead
func (c *TextToSpeechClient) CloseStream(ctx context.Context) error {
	if c.wsConn != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		if err := c.wsConn.WriteJSON(struct {
			Type string `json:"type"`
		}{
			Type: "Close",
		}); err != nil {
			log.Printf("Failed to send close message to deepgram websocket: %v", err)
		}

	}

	return nil
}

func (c *TextToSpeechClient) readAndProcessMessages(_ context.Context, conn *websocket.Conn, options texttospeech.TextToSpeechOptions) {
	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			if err.Error() != "websocket: close 1000 (normal)" {
				log.Printf("Websocket read error: %v", err)
			}

			c.wsConn.Close()
			c.wsConn = nil

			return
		}

		switch msgType {
		case websocket.BinaryMessage:
			if options.AudioCallback != nil && len(msg) > 0 {
				options.AudioCallback(msg)
			}
		default:
			var parsedMsg struct {
				Type string `json:"type"`
			}
			err := json.Unmarshal(msg, &parsedMsg)
			if err != nil {
				log.Printf("Failed to unmarshal deepgram message: %v", err)
				continue
			}

			switch parsedMsg.Type {
			case "Flushed":
				if len(c.transcriptBuffer) > 0 {
					if options.AudioEnded != nil {
						options.AudioEnded(c.transcriptBuffer[0])
					}
					c.transcriptBuffer = c.transcriptBuffer[1:]
				}
				if len(c.transcriptBuffer) > 0 {
					if err := c.speak(c.transcriptBuffer[0]); err != nil {
						log.Printf("Failed to speak deepgram text: %v", err)
						continue
					}
				}
				if len(c.transcriptBuffer) > 1 {
					if err := c.flush(); err != nil {
						log.Printf("Failed to flush deepgram buffer: %v", err)
						continue
					}
				}
			}
		}
	}
}
