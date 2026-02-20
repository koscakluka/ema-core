package orchestration

import (
	"reflect"

	"github.com/koscakluka/ema-core/core/audio"
)

// audioOutput normalizes legacy (v0) and callback-mark (v1) clients behind
// one facade used by orchestration turns.
//
// The facade is lightweight: it caches typed capabilities derived from base so
// per-turn code can route without repeated type assertions.
//
// A turn should use a Snapshot() so later runtime reconfiguration does not
// change behavior mid-turn.
//
// NOTE: methods intentionally do best-effort forwarding and ignore client
// return errors because the existing orchestration pipeline treats audio output
// as non-fatal side effects.
type audioOutput struct {

	// base stores the configured output client regardless of protocol version.
	base audioOutputBase
	// v0 is set when the output client supports the legacy mark-wait API.
	v0 AudioOutputV0
	// v1 is set when the output client supports callback-based mark handling.
	v1 AudioOutputV1

	// supportsCallbackMarks reports whether marks can invoke callbacks directly.
	supportsCallbackMarks bool
}

// newAudioOutput builds a facade and applies Set immediately so typed
// capabilities are computed once at construction.
func newAudioOutput(client audioOutputBase) *audioOutput {
	audioOutput := audioOutput{}
	audioOutput.Set(client)
	return &audioOutput
}

// Set replaces the configured output client and recomputes version-specific
// capabilities. Nil and typed-nil clients are treated as unconfigured.
func (a *audioOutput) Set(client audioOutputBase) {
	if a == nil {
		return
	}

	a.base = nil
	a.v0 = nil
	a.v1 = nil
	a.supportsCallbackMarks = false

	if isNilAudioOutputBase(client) {
		return
	}
	a.base = client

	if v1, ok := client.(AudioOutputV1); ok {
		a.v1 = v1
		a.supportsCallbackMarks = true
		return
	}

	if v0, ok := client.(AudioOutputV0); ok {
		a.v0 = v0
	}
}

// isConfigured reports whether the facade has a usable typed output client.
// This checks version-specific bindings instead of base so unsupported or
// typed-nil interface values are not considered configured.
func (a *audioOutput) isConfigured() bool {
	if a == nil {
		return false
	}

	return a.v0 != nil || a.v1 != nil
}

// Snapshot returns a per-turn copy of the facade state. The copy intentionally
// keeps the same underlying client instance while freezing protocol routing for
// the lifetime of the turn.
func (a *audioOutput) Snapshot() *audioOutput {
	if a == nil {
		return a
	}

	return newAudioOutput(a.base)
}

// SendAudio forwards a chunk to the configured output client.
//
// v1 is preferred when available; otherwise v0 is used. If no usable client is
// configured, the chunk is dropped.
func (a *audioOutput) SendAudio(audio []byte) {
	if a.v1 != nil {
		a.v1.SendAudio(audio)
	} else if a.v0 != nil {
		a.v0.SendAudio(audio)
	}
}

// Mark coordinates transcript marks with output playback.
//
// For v1 clients, mark handling is delegated directly.
// For v0 clients, AwaitMark is bridged to a callback so turn logic can stay
// callback-driven.
// Without output configured, the callback is invoked immediately so turn state
// can continue progressing.
func (a *audioOutput) Mark(mark string, callback func(string)) {
	if a.v1 != nil {
		a.v1.Mark(mark, callback)
	} else if a.v0 != nil {
		// Legacy outputs expose mark confirmation as a blocking wait. Run the
		// wait in a goroutine so mark handling does not block the caller.
		go func() {
			a.v0.AwaitMark()
			callback(mark)
		}()
	} else {
		callback(mark)
	}
}

// Clear flushes buffered output on the configured client.
//
// If no supported client is configured, this is a no-op.
func (a *audioOutput) Clear() {
	if a.v1 != nil {
		a.v1.ClearBuffer()
	} else if a.v0 != nil {
		a.v0.ClearBuffer()
	}
}

// EncodingInfo returns the active output encoding metadata.
//
// If no supported client is configured, the project default encoding is used.
func (a *audioOutput) EncodingInfo() audio.EncodingInfo {
	if a.v1 != nil {
		return a.v1.EncodingInfo()
	}
	if a.v0 != nil {
		return a.v0.EncodingInfo()
	}

	return audio.GetDefaultEncodingInfo()
}

// isNilAudioOutputBase detects nil and typed-nil interface values so Set can
// avoid storing unusable interface wrappers as configured clients.
func isNilAudioOutputBase(client audioOutputBase) bool {
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
