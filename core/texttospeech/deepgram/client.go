package deepgram

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/koscakluka/ema-core/core/texttospeech"
)

type TextToSpeechClient struct {
	wsConn            *websocket.Conn
	transcriptBuffer  []string
	postRestartBuffer []string
	options           texttospeech.TextToSpeechOptions

	voice deepgramVoice
	mu    sync.Mutex
}

func NewTextToSpeechClient(ctx context.Context, voice deepgramVoice) (*TextToSpeechClient, error) {
	client := &TextToSpeechClient{voice: defaultVoice}

	if !slices.Contains(GetAvailableVoices(), voice) {
		return nil, fmt.Errorf("invalid voice")
	}

	client.voice = voice

	return client, nil
}

func (c *TextToSpeechClient) Close(ctx context.Context) {
	c.CloseStream(ctx)
}

func (c *TextToSpeechClient) SetVoice(voice deepgramVoice) {
	c.voice = voice
}

func (c *TextToSpeechClient) Restart(ctx context.Context) error {
	if c.postRestartBuffer != nil {
		// We are already restarting, do nothing
		return nil
	}
	c.postRestartBuffer = []string{}
	_ = c.ClearBuffer()
	_ = c.CloseStream(ctx)
	_ = c.OpenStream(ctx)
	c.transcriptBuffer = c.postRestartBuffer
	c.postRestartBuffer = nil

	return nil
}
