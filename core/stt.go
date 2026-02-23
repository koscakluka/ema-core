package orchestration

import (
	"context"
	"fmt"

	"github.com/koscakluka/ema-core/core/audio"
	"github.com/koscakluka/ema-core/core/llms"
	"github.com/koscakluka/ema-core/core/speechtotext"
	"github.com/koscakluka/ema-core/core/triggers"
)

type speechToText struct {
	// client stores the configured speech-to-text implementation.
	client SpeechToText

	onSpeechStarted               func()
	onSpeechEnded                 func()
	onPartialInterimTranscription func(string)
	onInterimTranscription        func(string)
	onPartialTranscription        func(string)
	onTranscription               func(string)
	invokeTrigger                 func(llms.TriggerV0)
}

func newSpeechToText(client SpeechToText) *speechToText {
	return &speechToText{
		client:                        client,
		onSpeechStarted:               func() {},
		onSpeechEnded:                 func() {},
		onPartialInterimTranscription: func(string) {},
		onInterimTranscription:        func(string) {},
		onPartialTranscription:        func(string) {},
		onTranscription:               func(string) {},
		invokeTrigger:                 func(llms.TriggerV0) {},
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

func (s *speechToText) SetPartialInterimTranscriptionCallback(callback func(string)) {
	if s != nil {
		if callback != nil {
			s.onPartialInterimTranscription = callback
		} else {
			s.onPartialInterimTranscription = func(string) {}
		}
	}
}

func (s *speechToText) SetPartialTranscriptionCallback(callback func(string)) {
	if s != nil {
		if callback != nil {
			s.onPartialTranscription = callback
		} else {
			s.onPartialTranscription = func(string) {}
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

func (s *speechToText) SetInvokeTrigger(invokeTrigger func(llms.TriggerV0)) {
	if s != nil {
		if invokeTrigger != nil {
			s.invokeTrigger = invokeTrigger
		} else {
			s.invokeTrigger = func(llms.TriggerV0) {}
		}
	}
}

func (s *speechToText) isConfigured() bool {
	return s != nil && s.client != nil
}

func (s *speechToText) invokeSpeechStarted() {
	s.onSpeechStarted()
	go s.invokeTrigger(triggers.NewSpeechStartedTrigger())
}

func (s *speechToText) invokeSpeechEnded() {
	s.onSpeechEnded()
	go s.invokeTrigger(triggers.NewSpeechEndedTrigger())
}

func (s *speechToText) invokeInterimTranscription(transcript string) {
	s.onInterimTranscription(transcript)
	go s.invokeTrigger(triggers.NewInterimTranscriptionTrigger(transcript))
}

func (s *speechToText) invokePartialInterimTranscription(transcript string) {
	s.onPartialInterimTranscription(transcript)
}

func (s *speechToText) invokePartialTranscription(transcript string) {
	s.onPartialTranscription(transcript)
}

func (s *speechToText) invokeTranscription(transcript string) {
	s.onPartialInterimTranscription("")
	s.onInterimTranscription("")
	s.onTranscription(transcript)
	go s.invokeTrigger(triggers.NewTranscriptionTrigger(transcript))
}
