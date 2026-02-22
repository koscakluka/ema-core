package orchestration

import (
	"context"
	"fmt"

	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/events"
	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/core/speechtotext"
)

type speechToText struct {
	// client stores the configured speech-to-text implementation.
	client SpeechToText

	onSpeechStarted        func()
	onSpeechEnded          func()
	onInterimTranscription func(string)
	onTranscription        func(string)
	invokeEvent            func(llms.EventV0)
}

func newSpeechToText(client SpeechToText) *speechToText {
	return &speechToText{
		client:                 client,
		onSpeechStarted:        func() {},
		onSpeechEnded:          func() {},
		onInterimTranscription: func(string) {},
		onTranscription:        func(string) {},
		invokeEvent:            func(llms.EventV0) {},
	}
}

func (s *speechToText) set(client SpeechToText) {
	if s != nil {
		s.client = client
	}
}

func (s *speechToText) Start(ctx context.Context, encodingInfo *audio.EncodingInfo) error {
	if !s.isConfigured() {
		return nil
	}

	sttOptions := []speechtotext.TranscriptionOption{
		speechtotext.WithSpeechStartedCallback(s.invokeSpeechStarted),
		speechtotext.WithSpeechEndedCallback(s.invokeSpeechEnded),
		speechtotext.WithInterimTranscriptionCallback(s.invokeInterimTranscription),
		speechtotext.WithTranscriptionCallback(s.invokeTranscription),
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

func (s *speechToText) SetSpeechStateChangedCallback(callback func(bool)) {
	if s != nil {
		if callback != nil {
			s.onSpeechStarted = func() { callback(true) }
			s.onSpeechEnded = func() { callback(false) }
		} else {
			s.onSpeechStarted = func() {}
			s.onSpeechEnded = func() {}
		}
	}
}

func (s *speechToText) SetInterimTranscriptionCallback(callback func(string)) {
	if s != nil {
		if callback != nil {
			s.onInterimTranscription = callback
		} else {
			s.onInterimTranscription = func(string) {}
		}
	}
}

func (s *speechToText) SetTranscriptionCallback(callback func(string)) {
	if s != nil {
		if callback != nil {
			s.onTranscription = callback
		} else {
			s.onTranscription = func(string) {}
		}
	}
}

func (s *speechToText) SetInvokeEvent(invokeEvent func(llms.EventV0)) {
	if s != nil {
		if invokeEvent != nil {
			s.invokeEvent = invokeEvent
		} else {
			s.invokeEvent = func(llms.EventV0) {}
		}
	}
}

func (s *speechToText) isConfigured() bool {
	return s != nil && s.client != nil
}

func (s *speechToText) invokeSpeechStarted() {
	s.onSpeechStarted()
	go s.invokeEvent(events.NewSpeechStartedEvent())
}

func (s *speechToText) invokeSpeechEnded() {
	s.onSpeechEnded()
	go s.invokeEvent(events.NewSpeechEndedEvent())
}

func (s *speechToText) invokeInterimTranscription(transcript string) {
	s.onInterimTranscription(transcript)
	go s.invokeEvent(events.NewInterimTranscriptionEvent(transcript))
}

func (s *speechToText) invokeTranscription(transcript string) {
	s.onInterimTranscription("")
	s.onTranscription(transcript)
	go s.invokeEvent(events.NewTranscriptionEvent(transcript))
}
