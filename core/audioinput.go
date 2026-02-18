package orchestration

import (
	"context"
	"errors"
	"log"
	"sync/atomic"

	"github.com/koscakluka/ema-core/core/audio"
)

type audioInput struct {
	// base stores the configured input client used for streaming audio.
	base audioInputBase
	// fineCaptureControle is set when the input client supports explicit capture controls.
	fineCaptureControle AudioInputFine

	// connected reports whether a concrete input client is currently configured.
	connected atomic.Bool
	// isCapturing reports whether the input client is currently capturing audio.
	isCapturing atomic.Bool

	// alwaysCapture keeps capture running continuously when control APIs exist.
	alwaysCapture atomic.Bool
	// shouldCapture reports whether the input client should be capturing audio.
	shouldCapture atomic.Bool

	// onInputAudio is called when input audio is received
	onInputAudio func(audio []byte)
}

func newAudioInput(client audioInputBase, onInputAudio func(audio []byte)) *audioInput {
	if onInputAudio == nil {
		onInputAudio = func(audio []byte) {}
	}

	audioInput := audioInput{onInputAudio: onInputAudio}
	audioInput.alwaysCapture.Store(true)
	audioInput.Set(client)
	return &audioInput
}

func (a *audioInput) Set(client audioInputBase) {
	if a == nil {
		return
	}

	a.base = client
	a.fineCaptureControle = nil
	a.connected.Store(false)
	a.isCapturing.Store(false)

	if client == nil {
		return
	}

	a.connected.Store(true)
	if fine, ok := client.(AudioInputFine); ok {
		a.fineCaptureControle = fine
	}
}

func (a *audioInput) IsConfigured() bool            { return a != nil && a.connected.Load() }
func (a *audioInput) SupportsCaptureControls() bool { return a != nil && a.fineCaptureControle != nil }
func (a *audioInput) IsAlwaysRecording() bool       { return a == nil || a.alwaysCapture.Load() } // defaults to true
func (a *audioInput) IsCapturing() bool             { return a != nil && a.isCapturing.Load() }
func (a *audioInput) ShouldCapture() bool           { return a != nil && a.shouldCapture.Load() }

func (a *audioInput) EnableAlwaysCapture(ctx context.Context) error {
	if a == nil {
		return nil
	}

	a.alwaysCapture.Store(true)
	return a.Capture(ctx)
}

func (a *audioInput) DisableAlwaysCapture(context.Context) error {
	if a == nil {
		return nil
	}

	a.alwaysCapture.Store(false)
	return a.StopCapture()
}

func (a *audioInput) RequestCapture(ctx context.Context) error {
	if a == nil {
		return nil
	}

	a.shouldCapture.Store(true)
	return a.Capture(ctx)
}

func (a *audioInput) ReleaseCapture(context.Context) error {
	if a == nil {
		return nil
	}

	a.shouldCapture.Store(false)
	return a.StopCapture()
}

func (a *audioInput) Start(ctx context.Context) {
	if a.IsConfigured() {
		a.Capture(ctx)
	}
}

func (a *audioInput) Capture(ctx context.Context) error {
	if a == nil {
		return nil
	}

	if !a.isCapturing.CompareAndSwap(false, true) {
		return nil
	}

	if a.SupportsCaptureControls() {
		if a.IsAlwaysRecording() || a.ShouldCapture() {
			go func() {
				if err := a.fineCaptureControle.StartCapture(ctx, a.onAudio); err != nil {
					a.isCapturing.Store(false)
					// TODO: Find a way to propagate this error
					log.Printf("Failed to start audio input: %v", err)
				}
			}()
			return nil
		}

		a.isCapturing.Store(false)
		return nil
	}

	if a.base != nil {
		go func() {
			if err := a.base.Stream(ctx, a.onAudio); err != nil {
				a.isCapturing.Store(false)
				// TODO: Find a way to propagate this error
				log.Printf("Failed to start audio input: %v", err)
			}
		}()
		return nil
	}

	a.isCapturing.Store(false)
	return nil
}

func (a *audioInput) Close() error {
	var errs error
	if a.base != nil && a.IsConfigured() {
		if a.fineCaptureControle != nil {
			if err := a.fineCaptureControle.StopCapture(); err != nil {
				errs = errors.Join(errs, err)
			}
		}

		a.base.Close()
	}
	a.isCapturing.Store(false)

	return errs
}

func (a *audioInput) StopCapture() error {
	if a.SupportsCaptureControls() {
		if a.IsAlwaysRecording() || a.ShouldCapture() {
			return nil
		}

		if err := a.fineCaptureControle.StopCapture(); err != nil {
			return err
		}
		a.isCapturing.Store(false)
		return nil
	}

	return nil
}

func (a *audioInput) EncodingInfo() audio.EncodingInfo {
	if a == nil || a.base == nil {
		return audio.GetDefaultEncodingInfo()
	}

	return a.base.EncodingInfo()
}

func (a *audioInput) onAudio(audio []byte) {
	if !a.IsAlwaysRecording() && !a.ShouldCapture() {
		return
	}

	a.onInputAudio(audio)
}
