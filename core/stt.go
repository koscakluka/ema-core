package orchestration

import (
	"context"
	"fmt"

	"github.com/koscakluka/ema-core/core/audio"
	events "github.com/koscakluka/ema-core/core/events"
	"github.com/koscakluka/ema-core/core/speechtotext"
)

type speechToText struct {
	// client stores the configured speech-to-text implementation.
	client SpeechToText

	emitEvent eventEmitter
}

func newSpeechToText(client SpeechToText) *speechToText {
	return &speechToText{
		client:    client,
		emitEvent: noopEventEmitter,
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
		speechtotext.WithPartialInterimTranscriptionCallback(s.invokePartialInterimTranscription),
		speechtotext.WithInterimTranscriptionCallback(s.invokeInterimTranscription),
		speechtotext.WithPartialTranscriptionCallback(s.invokePartialTranscription),
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

func (s *speechToText) SetEventEmitter(emitEvent eventEmitter) {
	if s != nil {
		if emitEvent != nil {
			s.emitEvent = emitEvent
		} else {
			s.emitEvent = noopEventEmitter
		}
	}
}

func (s *speechToText) isConfigured() bool {
	return s != nil && s.client != nil
}

func (s *speechToText) invokeSpeechStarted() {
	s.emitEvent(events.NewUserSpeechStarted())
}

func (s *speechToText) invokeSpeechEnded() {
	s.emitEvent(events.NewUserSpeechEnded())
}

func (s *speechToText) invokeInterimTranscription(transcript string) {
	s.emitEvent(events.NewUserTranscriptInterimUpdated(transcript))
}

func (s *speechToText) invokePartialInterimTranscription(transcript string) {
	s.emitEvent(events.NewUserTranscriptInterimSegmentUpdated(transcript))
}

func (s *speechToText) invokePartialTranscription(transcript string) {
	s.emitEvent(events.NewUserTranscriptSegment(transcript))
}

func (s *speechToText) invokeTranscription(transcript string) {
	s.emitEvent(events.NewUserTranscriptInterimSegmentUpdated(""))
	s.emitEvent(events.NewUserTranscriptInterimUpdated(""))
	s.emitEvent(events.NewUserTranscriptFinal(transcript))
}
