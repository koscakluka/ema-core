package orchestration

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/koscakluka/ema-core/core/texttospeech"
)

type textToSpeech struct {
	// base stores whichever TTS implementation was configured for this turn.
	base textToSpeechBase

	// ttsClient is the legacy streaming TTS client (v0-style API).
	ttsClient TextToSpeech
	// ttsGenerator is the newer generator API used by v1-style clients.
	ttsGenerator texttospeech.SpeechGeneratorV0

	// initialized closes when init completes so workers can safely proceed.
	initialized chan struct{}
	// connected reports whether a TTS client/generator was initialized.
	connected bool
	// closeStarted makes Close idempotent under concurrent shutdown paths.
	closeStarted atomic.Bool
}

func newTextToSpeech(client textToSpeechBase) *textToSpeech {
	textToSpeech := textToSpeech{}
	textToSpeech.set(client)
	return &textToSpeech
}

func (t *textToSpeech) set(client textToSpeechBase) {
	if t == nil {
		return
	}
	t.base = client
}

func (t *textToSpeech) client() textToSpeechBase {
	if t == nil {
		return nil
	}

	return t.base
}

func (t *textToSpeech) init(ctx context.Context, turn *activeTurn) error {
	if t.initialized == nil {
		t.initialized = make(chan struct{})
	}
	defer close(t.initialized)

	ttsOptions := []texttospeech.TextToSpeechOption{
		texttospeech.WithSpeechAudioCallback(turn.audioBuffer.AddAudio),
		texttospeech.WithSpeechMarkCallback(turn.audioBuffer.Mark),
		texttospeech.WithEncodingInfo(turn.audioOutput.EncodingInfo()),
	}

	if t.base != nil {
		if client, ok := t.base.(TextToSpeechV1); ok {
			ttsOptions = append(ttsOptions, texttospeech.WithSpeechEndedCallbackV0(func(texttospeech.SpeechEndedReport) {
				turn.audioBuffer.AllAudioLoaded()
			}))

			speechGenerator, err := client.NewSpeechGeneratorV0(ctx, ttsOptions...)
			if err != nil {
				return fmt.Errorf("failed to create speech generator: %w", err)
			}
			t.ttsGenerator = speechGenerator
			t.connected = true
		} else if client, ok := t.base.(TextToSpeech); ok {
			if err := client.OpenStream(ctx, ttsOptions...); err != nil {
				return fmt.Errorf("failed to open tts stream: %w", err)
			}
			t.ttsClient = client
			t.connected = true
			turn.audioBuffer.usingWithLegacyTTS = true
		}
	}

	return nil
}

func (t *textToSpeech) waitUntilInitialized() {
	for t.initialized == nil {
		time.Sleep(10 * time.Millisecond)
	}
	<-t.initialized
}

func (t *textToSpeech) Close(ctx context.Context) error {
	if !t.closeStarted.CompareAndSwap(false, true) {
		return nil
	}

	var closeErr error
	closedAny := false

	if t.ttsClient != nil {
		closedAny = true
		switch client := t.ttsClient.(type) {
		case interface{ Close(context.Context) error }:
			if err := client.Close(ctx); err != nil {
				closeErr = errors.Join(closeErr, fmt.Errorf("legacy tts client close(ctx) failed: %w", err))
			}
		case interface{ Close(context.Context) }:
			client.Close(ctx)
		case interface{ CloseStream(context.Context) error }:
			if err := client.CloseStream(ctx); err != nil {
				closeErr = errors.Join(closeErr, fmt.Errorf("legacy tts client close stream failed: %w", err))
			}
		default:
			closeErr = errors.Join(closeErr, fmt.Errorf("legacy tts client does not expose a supported close method"))
		}
	}

	if t.ttsGenerator != nil {
		closedAny = true
		if err := t.ttsGenerator.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("speech generator close failed: %w", err))
		}
	}

	if !closedAny {
		return nil
	}

	if closeErr != nil {
		return fmt.Errorf("failed to close active turn tts resources: %w", closeErr)
	}

	return nil
}

func (t *textToSpeech) SendText(text string) error {
	if t.ttsClient != nil {
		if err := t.ttsClient.SendText(text); err != nil {
			return fmt.Errorf("failed to send text to tts: %w", err)
		}
		if t.ttsGenerator != nil {
			if err := t.ttsGenerator.SendText(text); err != nil {
				return fmt.Errorf("failed to send text to tts: %w", err)
			}
		}
	} else if t.ttsGenerator != nil {
		if err := t.ttsGenerator.SendText(text); err != nil {
			return fmt.Errorf("failed to send text to tts: %w", err)
		}
	}

	return nil
}

func (t *textToSpeech) Mark() error {
	if t.ttsClient != nil {
		if err := t.ttsClient.FlushBuffer(); err != nil {
			return fmt.Errorf("failed to send flush to tts: %w", err)
		}
	} else if t.ttsGenerator != nil {
		if err := t.ttsGenerator.Mark(); err != nil {
			return fmt.Errorf("failed to send mark to tts: %w", err)
		}
	}

	return nil
}

func (t *textToSpeech) EndOfText() error {
	if t.ttsClient != nil {
		if err := t.ttsClient.FlushBuffer(); err != nil {
			return fmt.Errorf("failed to send flush to tts: %w", err)
		}
	} else if t.ttsGenerator != nil {
		if err := t.ttsGenerator.Mark(); err != nil {
			return fmt.Errorf("failed to send flush to tts: %w", err)
		}
		if err := t.ttsGenerator.EndOfText(); err != nil {
			return fmt.Errorf("failed to send end of text to tts: %w", err)
		}
	}

	return nil
}

func (t *textToSpeech) Cancel() error {
	if t.ttsGenerator != nil {
		if err := t.ttsGenerator.Cancel(); err != nil {
			return fmt.Errorf("failed to cancel tts: %w", err)
		}
	}

	return nil
}
