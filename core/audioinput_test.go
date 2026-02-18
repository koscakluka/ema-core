package orchestration

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/koscakluka/ema-core/core/audio"
)

func TestWithAudioInputConfiguresAudioInputFacade(t *testing.T) {
	inputClient := &testAudioInputClient{}
	o := NewOrchestrator(WithAudioInput(inputClient))

	if !o.audioInput.IsConfigured() {
		t.Fatalf("expected audio input facade to be configured")
	}
	if o.audioInput.base != inputClient {
		t.Fatalf("expected facade client to match configured audio input")
	}
}

func TestAudioInputFacadeUsesDefaultEncodingInfoWhenUnset(t *testing.T) {
	facade := newTestAudioInput(nil)

	if facade.IsConfigured() {
		t.Fatalf("expected unset facade to be unconfigured")
	}

	if got, want := facade.EncodingInfo(), audio.GetDefaultEncodingInfo(); got != want {
		t.Fatalf("expected default encoding info %+v, got %+v", want, got)
	}
}

func TestAudioInputFacadeAlwaysRecordingDefaultsTrue(t *testing.T) {
	facade := newTestAudioInput(nil)

	if !facade.IsAlwaysRecording() {
		t.Fatalf("expected always recording to default to true")
	}
}

func TestAudioInputFacadeCaptureControlsNoopForBasicInput(t *testing.T) {
	facade := newTestAudioInput(&testAudioInputClient{})

	if facade.SupportsCaptureControls() {
		t.Fatalf("expected basic input to not support capture controls")
	}

	if err := facade.Capture(context.Background()); err != nil {
		t.Fatalf("expected start capture noop to succeed, got %v", err)
	}
	if err := facade.StopCapture(); err != nil {
		t.Fatalf("expected stop capture noop to succeed, got %v", err)
	}
}

func TestAudioInputFacadeCaptureForwardsInputAudio(t *testing.T) {
	inputClient := &testStreamingAudioInputClient{}
	var callbackCalls atomic.Int32
	facade := newAudioInput(inputClient, func([]byte) {
		callbackCalls.Add(1)
	})

	if err := facade.Capture(context.Background()); err != nil {
		t.Fatalf("expected capture to start, got %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if callbackCalls.Load() == 2 && inputClient.streamCalls.Load() == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf(
		"expected 2 callback invocations and 1 stream call, got callback calls=%d stream calls=%d",
		callbackCalls.Load(),
		inputClient.streamCalls.Load(),
	)
}

type testAudioInputClient struct{}

func newTestAudioInput(client audioInputBase) *audioInput {
	return newAudioInput(client, nil)
}

func (testAudioInputClient) EncodingInfo() audio.EncodingInfo {
	return audio.GetDefaultEncodingInfo()
}

func (testAudioInputClient) Stream(context.Context, func([]byte)) error {
	return nil
}

func (testAudioInputClient) Close() {}

type testFineAudioInputClient struct {
	testAudioInputClient
	startCaptureCalls  atomic.Int32
	stopCaptureCalls   atomic.Int32
	startCaptureCalled chan struct{}
}

func (c *testFineAudioInputClient) StartCapture(context.Context, func([]byte)) error {
	c.startCaptureCalls.Add(1)
	if c.startCaptureCalled != nil {
		select {
		case c.startCaptureCalled <- struct{}{}:
		default:
		}
	}
	return nil
}

func (c *testFineAudioInputClient) StopCapture() error {
	c.stopCaptureCalls.Add(1)
	return nil
}

type testStreamingAudioInputClient struct {
	testAudioInputClient
	streamCalls atomic.Int32
}

func (c *testStreamingAudioInputClient) Stream(_ context.Context, onAudio func([]byte)) error {
	c.streamCalls.Add(1)
	onAudio([]byte{0x01})
	onAudio([]byte{0x02})
	return nil
}
