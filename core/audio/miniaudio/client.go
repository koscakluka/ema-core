package miniaudio

import (
	"context"
	"fmt"
	"log"

	"github.com/gen2brain/malgo"
	"github.com/koscakluka/ema-core/core/audio"
)

const sampleRate = 48000

type Client struct {
	// audioContext is only saved to be able to uninitialize it, it is an
	// ownership thing
	audioContext *malgo.AllocatedContext
	playbackClient
	captureClient
}

func NewClient() (*Client, error) {
	audioCtx, err := malgo.InitContext(
		nil,
		malgo.ContextConfig{},
		func(message string) {}, //log.Println("malgo:", message) },
	)
	if err != nil {
		log.Fatalf("malgo InitContext failed: %v", err)
	}

	client := Client{
		audioContext: audioCtx,
	}

	if err := client.playbackClient.Init(audioCtx); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to initialize playback client: %w", err)
	}

	// TODO: This should probably start later
	if err := client.playbackClient.Start(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to start playback device: %w", err)
	}

	if err := client.captureClient.Init(audioCtx); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to initialize capture client: %w", err)
	}

	return &client, nil
}

func (c *Client) Stream(_ context.Context, onAudio func(audio []byte)) error {
	return c.captureClient.Start(onAudio)
}

func (c *Client) StartCapture(_ context.Context, onAudio func(audio []byte)) error {
	return c.captureClient.Start(onAudio)
}

func (c *Client) StopCapture() error {
	return c.captureClient.Stop()
}

func (c *Client) StartPlayback(_ context.Context) error {
	return c.playbackClient.Start()
}

func (c *Client) StopPlayback() error {
	return c.playbackClient.Stop()
}

func (c *Client) Close() {
	_ = c.captureClient.Uninit()
	_ = c.playbackClient.Uninit()
	_ = c.audioContext.Uninit()
	c.audioContext.Free()
}

func (c *Client) SendAudio(audio []byte) error {
	return c.playbackClient.SendAudio(audio)
}

func (c *Client) ClearBuffer() {
	c.playbackClient.ClearBuffer()
}

func (c *Client) AwaitMark() error {
	return c.playbackClient.AwaitMark()
}

func (c *Client) EncodingInfo() audio.EncodingInfo {
	return audio.EncodingInfo{
		SampleRate: sampleRate,
		Encoding:   "linear16",
	}
}
