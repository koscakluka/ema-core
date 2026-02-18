package orchestration

import (
	"context"
	"fmt"

	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/speechtotext"
)

type speechToTextCallbacks struct {
	onSpeechStarted        func()
	onSpeechEnded          func()
	onInterimTranscription func(string)
	onTranscription        func(string)
}

type speechToText struct {
	// client stores the configured speech-to-text implementation.
	client SpeechToText
}

func (s *speechToText) set(client SpeechToText) {
	if s == nil {
		return
	}
	s.client = client
}

func (s *speechToText) isConfigured() bool {
	return s != nil && s.client != nil
}

func (s *speechToText) start(ctx context.Context, callbacks speechToTextCallbacks, encodingInfo *audio.EncodingInfo) error {
	if !s.isConfigured() {
		return nil
	}

	sttOptions := []speechtotext.TranscriptionOption{}
	if callbacks.onSpeechStarted != nil {
		sttOptions = append(sttOptions, speechtotext.WithSpeechStartedCallback(callbacks.onSpeechStarted))
	}
	if callbacks.onSpeechEnded != nil {
		sttOptions = append(sttOptions, speechtotext.WithSpeechEndedCallback(callbacks.onSpeechEnded))
	}
	if callbacks.onInterimTranscription != nil {
		sttOptions = append(sttOptions, speechtotext.WithInterimTranscriptionCallback(callbacks.onInterimTranscription))
	}
	if callbacks.onTranscription != nil {
		sttOptions = append(sttOptions, speechtotext.WithTranscriptionCallback(callbacks.onTranscription))
	}
	if encodingInfo != nil {
		sttOptions = append(sttOptions, speechtotext.WithEncodingInfo(*encodingInfo))
	}

	if err := s.Transcribe(ctx, sttOptions...); err != nil {
		return fmt.Errorf("failed to start transcribing: %w", err)
	}

	return nil
}

func (s *speechToText) Transcribe(ctx context.Context, opts ...speechtotext.TranscriptionOption) error {
	if !s.isConfigured() {
		return nil
	}

	return s.client.Transcribe(ctx, opts...)
}

func (s *speechToText) SendAudio(audio []byte) error {
	if !s.isConfigured() {
		return nil
	}

	return s.client.SendAudio(audio)
}

func (s *speechToText) Close(ctx context.Context) error {
	if !s.isConfigured() {
		return nil
	}

	switch c := s.client.(type) {
	case interface{ Close(context.Context) error }:
		if err := c.Close(ctx); err != nil {
			return fmt.Errorf("failed to close speech-to-text client: %w", err)
		}
	case interface{ Close(context.Context) }:
		c.Close(ctx)
	case interface{ Close() error }:
		if err := c.Close(); err != nil {
			return fmt.Errorf("failed to close speech-to-text client: %w", err)
		}
	case interface{ Close() }:
		c.Close()
	}

	return nil
}
