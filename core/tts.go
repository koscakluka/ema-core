package orchestration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/koscakluka/ema-core/core/audio"
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
	// initOnce ensures per-turn initialization is executed once.
	initOnce sync.Once
	// initErr stores the one-time initialization result.
	initErr error

	clientMu sync.RWMutex
	// connected reports whether a TTS client/generator was initialized.
	connected atomic.Bool
	// closeStarted makes Close idempotent under concurrent shutdown paths.
	closeStarted atomic.Bool

	// isMuted indicates whether the TTS client is currently passing speech to
	// audio output.
	isMuted atomic.Bool

	// onAudio is called when a speech chunk is forwarded to output processing.
	onAudio func([]byte)
}

func newTextToSpeech(client textToSpeechBase, isMuted bool) *textToSpeech {
	textToSpeech := textToSpeech{
		initialized: make(chan struct{}),
		onAudio:     func([]byte) {},
	}
	textToSpeech.isMuted.Store(isMuted)
	textToSpeech.set(client)
	return &textToSpeech
}

func (t *textToSpeech) set(client textToSpeechBase) {
	if t == nil {
		return
	}
	t.base = client
}

func (t *textToSpeech) Snapshot() *textToSpeech {
	if t == nil {
		return t
	}

	snapshot := newTextToSpeech(t.base, t.isMuted.Load())
	snapshot.SetCallbacks(t.onAudio)
	return snapshot
}

func (t *textToSpeech) SetCallbacks(onAudio func([]byte)) {
	if t == nil {
		return
	}

	if onAudio != nil {
		t.onAudio = onAudio
	}
}

func (t *textToSpeech) init(ctx context.Context, speechPlayer *speechPlayer, encodingInfo audio.EncodingInfo) error {
	if t == nil {
		return nil
	}

	t.initOnce.Do(func() {
		defer close(t.initialized)
		t.connected.Store(false)
		if t.closeStarted.Load() {
			return
		}

		ttsOptions := []texttospeech.TextToSpeechOption{
			texttospeech.WithSpeechAudioCallback(func(audio []byte) {
				speechPlayer.AddAudioChunk(audio)
				t.onAudio(audio)
			}),
			texttospeech.WithSpeechMarkCallback(speechPlayer.AddAudioMark),
			texttospeech.WithEncodingInfo(encodingInfo),
		}

		if t.base != nil {
			if client, ok := t.base.(TextToSpeechV1); ok {
				ttsOptions = append(ttsOptions, texttospeech.WithSpeechEndedCallbackV0(func(texttospeech.SpeechEndedReport) {
					speechPlayer.AllAudioLoaded()
				}))

				speechGenerator, err := client.NewSpeechGeneratorV0(ctx, ttsOptions...)
				if err != nil {
					t.initErr = fmt.Errorf("failed to create speech generator: %w", err)
					return
				}
				if t.closeStarted.Load() {
					_ = speechGenerator.Close()
					return
				}
				t.clientMu.Lock()
				if t.closeStarted.Load() {
					t.clientMu.Unlock()
					_ = speechGenerator.Close()
					return
				}
				t.ttsGenerator = speechGenerator
				t.clientMu.Unlock()
				t.connected.Store(true)
				return
			}

			if client, ok := t.base.(TextToSpeech); ok {
				if err := client.OpenStream(ctx, ttsOptions...); err != nil {
					t.initErr = fmt.Errorf("failed to open tts stream: %w", err)
					return
				}
				if t.closeStarted.Load() {
					if err := closeLegacyTTSClient(ctx, client); err != nil {
						t.initErr = errors.Join(t.initErr, err)
					}
					return
				}
				t.clientMu.Lock()
				if t.closeStarted.Load() {
					t.clientMu.Unlock()
					if err := closeLegacyTTSClient(ctx, client); err != nil {
						t.initErr = errors.Join(t.initErr, err)
					}
					return
				}
				t.ttsClient = client
				t.clientMu.Unlock()
				t.connected.Store(true)
				speechPlayer.EnableLegacyTTSMode()
			}
		}
	})

	return t.initErr
}

func (t *textToSpeech) waitUntilInitialized(ctx context.Context) bool {
	if t != nil && t.initialized != nil {
		select {
		case <-t.initialized:
			return t.connected.Load()
		case <-ctx.Done():
			return false
		}
	}
	return false
}

func (t *textToSpeech) Close(ctx context.Context) error {
	if t == nil {
		return nil
	}

	if !t.closeStarted.CompareAndSwap(false, true) {
		return nil
	}

	var closeErr error
	closedAny := false

	t.clientMu.Lock()
	ttsClient := t.ttsClient
	ttsGenerator := t.ttsGenerator
	t.ttsClient = nil
	t.ttsGenerator = nil
	t.connected.Store(false)
	t.clientMu.Unlock()

	if ttsClient != nil {
		closedAny = true
		if err := closeLegacyTTSClient(ctx, ttsClient); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}

	if ttsGenerator != nil {
		closedAny = true
		if err := ttsGenerator.Close(); err != nil {
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

func closeLegacyTTSClient(ctx context.Context, client TextToSpeech) error {
	switch c := client.(type) {
	case interface{ Close(context.Context) error }:
		if err := c.Close(ctx); err != nil {
			return fmt.Errorf("legacy tts client close(ctx) failed: %w", err)
		}
	case interface{ Close(context.Context) }:
		c.Close(ctx)
	case interface{ CloseStream(context.Context) error }:
		if err := c.CloseStream(ctx); err != nil {
			return fmt.Errorf("legacy tts client close stream failed: %w", err)
		}
	default:
		return fmt.Errorf("legacy tts client does not expose a supported close method")
	}

	return nil
}

func (t *textToSpeech) SendText(text string) error {
	if t == nil {
		return nil
	}

	t.clientMu.RLock()
	ttsClient := t.ttsClient
	ttsGenerator := t.ttsGenerator
	t.clientMu.RUnlock()

	if ttsClient != nil {
		if err := ttsClient.SendText(text); err != nil {
			return fmt.Errorf("failed to send text to tts: %w", err)
		}
		if ttsGenerator != nil {
			if err := ttsGenerator.SendText(text); err != nil {
				return fmt.Errorf("failed to send text to tts: %w", err)
			}
		}
	} else if ttsGenerator != nil {
		if err := ttsGenerator.SendText(text); err != nil {
			return fmt.Errorf("failed to send text to tts: %w", err)
		}
	}

	return nil
}

func (t *textToSpeech) Mark() error {
	if t == nil {
		return nil
	}

	t.clientMu.RLock()
	ttsClient := t.ttsClient
	ttsGenerator := t.ttsGenerator
	t.clientMu.RUnlock()

	if ttsClient != nil {
		if err := ttsClient.FlushBuffer(); err != nil {
			return fmt.Errorf("failed to send flush to tts: %w", err)
		}
	} else if ttsGenerator != nil {
		if err := ttsGenerator.Mark(); err != nil {
			return fmt.Errorf("failed to send mark to tts: %w", err)
		}
	}

	return nil
}

func (t *textToSpeech) EndOfText() error {
	if t == nil {
		return nil
	}

	t.clientMu.RLock()
	ttsClient := t.ttsClient
	ttsGenerator := t.ttsGenerator
	t.clientMu.RUnlock()

	if ttsClient != nil {
		if err := ttsClient.FlushBuffer(); err != nil {
			return fmt.Errorf("failed to send flush to tts: %w", err)
		}
	} else if ttsGenerator != nil {
		if err := ttsGenerator.Mark(); err != nil {
			return fmt.Errorf("failed to send flush to tts: %w", err)
		}
		if err := ttsGenerator.EndOfText(); err != nil {
			return fmt.Errorf("failed to send end of text to tts: %w", err)
		}
	}

	return nil
}

func (t *textToSpeech) Cancel() error {
	if t == nil {
		return nil
	}

	t.clientMu.RLock()
	ttsGenerator := t.ttsGenerator
	t.clientMu.RUnlock()

	if ttsGenerator != nil {
		if err := ttsGenerator.Cancel(); err != nil {
			return fmt.Errorf("failed to cancel tts: %w", err)
		}
	}

	return nil
}

func (t *textToSpeech) IsMuted() bool { return t != nil && t.isMuted.Load() }

func (t *textToSpeech) Mute() error {
	if t != nil {
		t.isMuted.Store(true)
	}
	return nil
}

func (t *textToSpeech) Unmute() error {
	if t != nil {
		t.isMuted.Store(false)
	}
	return nil
}
