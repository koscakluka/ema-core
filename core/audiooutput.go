package orchestration

import (
	"reflect"

	"github.com/koscakluka/ema-core/core/audio"
)

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

func newAudioOutput(client audioOutputBase) *audioOutput {
	audioOutput := audioOutput{}
	audioOutput.Set(client)
	return &audioOutput
}

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

func (a *audioOutput) isConfigured() bool {
	if a == nil {
		return false
	}

	return a.v0 != nil || a.v1 != nil
}

func (a *audioOutput) Snapshot() audioOutput {
	if a == nil {
		return audioOutput{}
	}

	return *newAudioOutput(a.base)
}

func (a *audioOutput) SendAudio(audio []byte) {
	if a.v1 != nil {
		a.v1.SendAudio(audio)
	} else if a.v0 != nil {
		a.v0.SendAudio(audio)
	}
}

func (a *audioOutput) Mark(mark string, callback func(string)) {
	if a.v1 != nil {
		a.v1.Mark(mark, callback)
	} else if a.v0 != nil {
		go func() {
			a.v0.AwaitMark()
			callback(mark)
		}()
	} else {
		callback(mark)
	}
}

func (a *audioOutput) Clear() {
	if a.v1 != nil {
		a.v1.ClearBuffer()
	} else if a.v0 != nil {
		a.v0.ClearBuffer()
	}
}

func (a *audioOutput) EncodingInfo() audio.EncodingInfo {
	if a.v1 != nil {
		return a.v1.EncodingInfo()
	}
	if a.v0 != nil {
		return a.v0.EncodingInfo()
	}

	return audio.GetDefaultEncodingInfo()
}

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
