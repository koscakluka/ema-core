package orchestration

import (
	"context"
	"fmt"

	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/speechtotext"
)

type speechToTextCallbacks struct {
	onSpeechStateChanged   func(bool)
	onInterimTranscription func(string)
	onTranscription        func(string)
}

type speechToText struct {
	// client stores the configured speech-to-text implementation.
	client SpeechToText

	onSpeechStarted        func()
	onSpeechEnded          func()
	onInterimTranscription func(string)
	onTranscription        func(string)
}

func newSpeechToText(client SpeechToText) *speechToText {
	return &speechToText{
		client:                 client,
		onSpeechStarted:        func() {},
		onSpeechEnded:          func() {},
		onInterimTranscription: func(string) {},
		onTranscription:        func(string) {},
	}
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

func (s *speechToText) SetCallbacks(callbacks speechToTextCallbacks) {
	if s == nil {
		return
	}

	if callbacks.onSpeechStateChanged != nil {
		s.onSpeechStarted = func() {
			callbacks.onSpeechStateChanged(true)
		}
		s.onSpeechEnded = func() {
			callbacks.onSpeechStateChanged(false)
		}
	}
	if callbacks.onInterimTranscription != nil {
		s.onInterimTranscription = callbacks.onInterimTranscription
	}
	if callbacks.onTranscription != nil {
		s.onTranscription = callbacks.onTranscription
	}
}

func (s *speechToText) start(ctx context.Context, encodingInfo *audio.EncodingInfo) error {
	if !s.isConfigured() {
		return nil
	}

	// TODO: These need to be swipe-outable so they should probably be set instead of passed in start
	sttOptions := []speechtotext.TranscriptionOption{
		speechtotext.WithSpeechStartedCallback(s.onSpeechStarted),
		speechtotext.WithSpeechEndedCallback(s.onSpeechEnded),
		speechtotext.WithInterimTranscriptionCallback(s.onInterimTranscription),
		speechtotext.WithTranscriptionCallback(func(transcript string) {
			s.onInterimTranscription("")
			s.onTranscription(transcript)
		}),
		speechtotext.WithEncodingInfo(*encodingInfo),
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
