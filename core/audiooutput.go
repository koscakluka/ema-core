package orchestration

import "github.com/koscakluka/ema-core/core/audio"

type audioOutput struct {
	// base stores the configured output client regardless of protocol version.
	base audioOutputBase
	// v0 is set when the output client supports the legacy mark-wait API.
	v0 AudioOutputV0
	// v1 is set when the output client supports callback-based mark handling.
	v1 AudioOutputV1

	// connected reports whether a usable output client is configured.
	connected bool
	// supportsCallbackMarks reports whether marks can invoke callbacks directly.
	supportsCallbackMarks bool
	// isV1 tracks whether the configured client uses the v1 output contract.
	isV1 bool
}

func newAudioOutput(client audioOutputBase) *audioOutput {
	audioOutput := audioOutput{}
	audioOutput.set(client)
	return &audioOutput
}

func (a *audioOutput) set(client audioOutputBase) {
	if a == nil {
		return
	}

	a.base = client
	a.v0 = nil
	a.v1 = nil
	a.connected = false
	a.supportsCallbackMarks = false
	a.isV1 = false

	if client == nil {
		return
	}

	if v1, ok := client.(AudioOutputV1); ok {
		a.v1 = v1
		a.isV1 = true
		a.supportsCallbackMarks = true
		a.connected = true
		return
	}

	if v0, ok := client.(AudioOutputV0); ok {
		a.v0 = v0
		a.connected = true
	}
}

func (a *audioOutput) client() audioOutputBase {
	if a == nil {
		return nil
	}

	return a.base
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
