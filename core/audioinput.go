package orchestration

import (
	"context"
	"errors"
	"log"
	"reflect"
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

// newAudioInput creates an audioInput wrapper around the provided client.
//
// If no callback is provided, a no-op callback is used so callers do not need
// to guard against nil. Capture defaults to always-on mode.
func newAudioInput(client audioInputBase, onInputAudio func(audio []byte)) *audioInput {
	if onInputAudio == nil {
		onInputAudio = func(audio []byte) {}
	}

	audioInput := audioInput{onInputAudio: onInputAudio}
	audioInput.alwaysCapture.Store(true)
	audioInput.Set(client)
	return &audioInput
}

// Set replaces the active input client and re-detects optional capabilities.
//
// Any previous capture state is cleared because the previous client may no
// longer be valid.
func (a *audioInput) Set(client audioInputBase) {
	if a == nil {
		return
	}

	a.base = nil
	a.fineCaptureControle = nil
	a.connected.Store(false)
	a.isCapturing.Store(false)

	if isNilAudioInputBase(client) {
		return
	}

	a.base = client
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

// EnableAlwaysCapture keeps input capture running regardless of turn state.
func (a *audioInput) EnableAlwaysCapture(ctx context.Context) error {
	if a == nil {
		return nil
	}

	a.alwaysCapture.Store(true)
	return a.Capture(ctx)
}

// DisableAlwaysCapture disables continuous capture and attempts to stop input.
func (a *audioInput) DisableAlwaysCapture(context.Context) error {
	if a == nil {
		return nil
	}

	a.alwaysCapture.Store(false)
	return a.StopCapture()
}

// RequestCapture marks capture as needed for the current phase and starts it.
func (a *audioInput) RequestCapture(ctx context.Context) error {
	if a == nil {
		return nil
	}

	a.shouldCapture.Store(true)
	return a.Capture(ctx)
}

// ReleaseCapture clears transient capture intent and may stop capture.
func (a *audioInput) ReleaseCapture(context.Context) error {
	if a == nil {
		return nil
	}

	a.shouldCapture.Store(false)
	return a.StopCapture()
}

// Start initializes capture when a client is configured.
func (a *audioInput) Start(ctx context.Context) {
	if a.IsConfigured() {
		a.Capture(ctx)
	}
}

// Capture starts audio input exactly once per active capture session.
//
// For clients with fine-grained capture controls, capture starts only when the
// policy requires it (always-on or explicitly requested). For basic streaming
// clients, streaming starts whenever a client is configured.
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

// Close stops capture when supported and closes the configured client.
//
// Stop errors are accumulated and returned so cleanup can proceed even if one
// step fails.
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

// StopCapture stops capture only when no active policy requires it.
//
// For clients without explicit capture controls, capture lifecycle is managed
// externally by the stream context and there is nothing to stop here.
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

// EncodingInfo returns input encoding metadata or the package defaults.
func (a *audioInput) EncodingInfo() audio.EncodingInfo {
	if a == nil || a.base == nil {
		return audio.GetDefaultEncodingInfo()
	}

	return a.base.EncodingInfo()
}

// onAudio forwards captured audio only when current capture policy allows it.
func (a *audioInput) onAudio(audio []byte) {
	if !a.IsAlwaysRecording() && !a.ShouldCapture() {
		return
	}

	a.onInputAudio(audio)
}

func isNilAudioInputBase(client audioInputBase) bool {
	if client == nil {
		return true
	}

	v := reflect.ValueOf(client)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
