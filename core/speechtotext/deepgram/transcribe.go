package deepgram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	api "github.com/deepgram/deepgram-go-sdk/pkg/api/listen/v1/websocket/interfaces"
	"github.com/gorilla/websocket"
	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/speechtotext"
	"github.com/koscakluka/ema-core/internal/utils"
)

func (s *TranscriptionClient) Transcribe(ctx context.Context, opts ...speechtotext.TranscriptionOption) error {
	options := &speechtotext.TranscriptionOptions{EncodingInfo: audio.GetDefaultEncodingInfo()}
	for _, opt := range opts {
		opt(options)
	}

	encoding, err := convertEncoding(options.EncodingInfo)
	if err != nil {
		return fmt.Errorf("invalid encoding: %w", err)
	}

	conn, err := connectWebsocket(connectionOptions{
		sampleRate: encoding.SampleRate,
		encoding:   encoding.Format.Name(),

		detectSpeechStart: options.SpeechStartedCallback != nil,
		enhanceSpeechEndingDetection: options.TranscriptionCallback != nil ||
			options.SpeechEndedCallback != nil,
		interimResults: options.InterimTranscriptionCallback != nil,
	})
	if err != nil {
		return fmt.Errorf("failed to open websocket: %w", err)
	}

	s.conn = conn
	go s.readAndProcessMessages(ctx, conn, *options)

	return nil
}

type connectionOptions struct {
	sampleRate int
	encoding   string

	detectSpeechStart            bool
	enhanceSpeechEndingDetection bool
	interimResults               bool
}

func connectWebsocket(options connectionOptions) (*websocket.Conn, error) {
	apiKey, ok := os.LookupEnv("DEEPGRAM_API_KEY")
	if !ok {
		return nil, fmt.Errorf("deepgram api key not found")
	}

	listenUrl, _ := url.Parse("wss://api.deepgram.com/v1/listen")
	queryParams := listenUrl.Query()
	queryParams.Set("encoding", options.encoding)
	queryParams.Set("sample_rate", strconv.Itoa(options.sampleRate))
	queryParams.Set("channels", "1")
	queryParams.Set("model", "nova-3")
	queryParams.Set("language", "en-US")
	queryParams.Set("smart_format", "true")
	if options.enhanceSpeechEndingDetection {
		queryParams.Set("utterance_end_ms", "1000")
		queryParams.Set("interim_results", "true")
	} else if options.interimResults {
		queryParams.Set("interim_results", "true")
	}
	queryParams.Set("endpointing", "300")
	if options.detectSpeechStart || options.enhanceSpeechEndingDetection {
		queryParams.Set("vad_events", "true")
	}

	listenUrl.RawQuery = queryParams.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(listenUrl.String(),
		http.Header{"Authorization": {"Token " + apiKey}})
	if err != nil {
		return nil, fmt.Errorf("failed to open socket connection to deepgram: %w", err)
	}

	return conn, err
}

func (s *TranscriptionClient) sendKeepAlive() {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	if err := s.conn.WriteJSON(
		struct {
			Type string `json:"type"`
		}{
			Type: "KeepAlive",
		}); err != nil {
		log.Println("Failed to write to deepgram client", "error", err)
	}
}

func (s *TranscriptionClient) SendAudio(audio []byte) error {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	s.lastMsgTs = time.Now()
	if err := s.conn.WriteMessage(websocket.BinaryMessage, audio); err != nil {
		return fmt.Errorf("failed to write to deepgram client: %w", err)
	}
	return nil
}

func (s *TranscriptionClient) sendSilence(audio []byte) error {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	if err := s.conn.WriteMessage(websocket.BinaryMessage, audio); err != nil {
		return fmt.Errorf("failed to write to deepgram client: %w", err)
	}
	return nil
}

func (s *TranscriptionClient) StopStream() error {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	if s.conn != nil {
		if err := s.conn.WriteJSON(struct {
			Type string `json:"type"`
		}{Type: string(api.TypeCloseStreamResponse)}); err != nil {
			return fmt.Errorf("failed to clear deepgram buffer through websocket: %w", err)
		}
	}
	return nil
}

func (s *TranscriptionClient) readAndProcessMessages(ctx context.Context, conn *websocket.Conn, options speechtotext.TranscriptionOptions) {
	silenceCtx, silenceCancel := context.WithCancel(ctx)
	defer silenceCancel()

	go s.generateSilence(silenceCtx, options.EncodingInfo)

	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			if err.Error() != "websocket: close 1000 (normal)" {
				log.Println("Failed to read deepgram websocket message", "error")
			}

			s.conn = nil
			conn.Close()
			return
		}
		if msgType != websocket.BinaryMessage {
			go s.processMessage(ctx, msg, options)
		}
	}
}

func (s *TranscriptionClient) processMessage(_ context.Context, msg []byte, options speechtotext.TranscriptionOptions) {
	var parsedMsg struct {
		Type string `json:"type"`
	}
	err := json.Unmarshal(msg, &parsedMsg)
	if err != nil {
		log.Println("Failed to unmarshal deepgram message", "error", err)
		return
	}

	switch api.TypeResponse(parsedMsg.Type) {
	case api.TypeMessageResponse:
		var msgResp api.MessageResponse
		if err := json.Unmarshal(msg, &msgResp); err != nil {
			log.Println("Failed to unmarshal deepgram message", err)
			return
		}
		if msgResp.IsFinal {
			if len(msgResp.Channel.Alternatives) > 0 {
				transcript := strings.TrimSpace(msgResp.Channel.Alternatives[0].Transcript)
				if len(transcript) > 0 {
					if options.TranscriptionCallback != nil {
						s.accumulatedTranscript += " " + transcript
					}
					if options.PartialTranscriptionCallback != nil {
						options.PartialTranscriptionCallback(transcript)
					}
				}
			}
			if msgResp.SpeechFinal {
				s.onSpeechEnded(options)
			}
		}
		if !msgResp.IsFinal &&
			(options.PartialInterimTranscriptionCallback != nil || options.InterimTranscriptionCallback != nil) {
			if len(msgResp.Channel.Alternatives) > 0 {
				transcript := strings.TrimSpace(msgResp.Channel.Alternatives[0].Transcript)
				if len(transcript) > 0 {
					if options.PartialInterimTranscriptionCallback != nil {
						options.PartialInterimTranscriptionCallback(transcript)
					} else if options.InterimTranscriptionCallback != nil {
						options.InterimTranscriptionCallback(s.accumulatedTranscript + " " + transcript)
					}
				}
			}
		}

	case api.TypeUtteranceEndResponse:
		var msgResp api.UtteranceEndResponse
		if err := json.Unmarshal(msg, &msgResp); err != nil {
			log.Println("Failed to unmarshal deepgram message", err)
			return
		}

		if s.unendedSegment {
			s.onSpeechEnded(options)
		}
	case api.TypeSpeechStartedResponse:
		var msgResp api.SpeechStartedResponse
		if err := json.Unmarshal(msg, &msgResp); err != nil {
			log.Println("Failed to unmarshal deepgram message", err)
			return
		}

		s.unendedSegment = true
		if options.SpeechStartedCallback != nil {
			options.SpeechStartedCallback()
		}
	}

}

func (s *TranscriptionClient) onSpeechEnded(options speechtotext.TranscriptionOptions) {
	s.unendedSegment = false
	if options.TranscriptionCallback != nil {
		fullTranscript := strings.TrimSpace(s.accumulatedTranscript)
		s.accumulatedTranscript = ""
		if len(fullTranscript) > 0 {
			options.TranscriptionCallback(fullTranscript)
		}
	}
	if options.SpeechEndedCallback != nil {
		options.SpeechEndedCallback()
	}
}

func (s *TranscriptionClient) generateSilence(ctx context.Context, encoding audio.EncodingInfo) {
	type silenceGeneratorState string
	const (
		silenceGeneratorStateWaiting   silenceGeneratorState = "waiting"
		silenceGeneratorStateSilence   silenceGeneratorState = "silence"
		silenceGeneratorStateKeepAlive silenceGeneratorState = "keepAlive"
	)

	const durationMs = 50
	const milisecondsPerSecond = 1000
	ticker := time.NewTicker(durationMs * time.Millisecond)

	chunk := make([]byte, encoding.SampleRate*encoding.Format.ByteSize()*durationMs/milisecondsPerSecond)
	for i := range chunk {
		chunk[i] = encoding.SilenceValue()
	}

	var state = silenceGeneratorStateWaiting
	var firstSilenceTime *time.Time
	var lastKeepAliveTime *time.Time
	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
			switch state {
			case silenceGeneratorStateWaiting:
				if time.Since(s.lastMsgTs).Milliseconds() > 50 {
					state = silenceGeneratorStateSilence
					firstSilenceTime = utils.Ptr(time.Now())
					continue
				}

			case silenceGeneratorStateSilence:
				if time.Since(s.lastMsgTs).Milliseconds() < 50 {
					state = silenceGeneratorStateWaiting
					firstSilenceTime = nil
					continue
				}
				if time.Since(*firstSilenceTime).Milliseconds() >= 1000 {
					state = silenceGeneratorStateKeepAlive
					lastKeepAliveTime = utils.Ptr(time.Now())
					firstSilenceTime = nil
					continue
				}

				if err := s.sendSilence(chunk); err != nil {
					log.Println("Sending silence audio error", err)
				}

			case silenceGeneratorStateKeepAlive:
				if time.Since(s.lastMsgTs).Milliseconds() < 50 {
					state = silenceGeneratorStateWaiting
					continue
				}

				if time.Since(*lastKeepAliveTime).Seconds() >= 5 {
					lastKeepAliveTime = utils.Ptr(time.Now())
					s.sendKeepAlive()
				}
			}
		}
	}
}
