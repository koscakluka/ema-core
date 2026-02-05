package miniaudio

import (
	"fmt"
	"sync"

	"github.com/gen2brain/malgo"
	"github.com/koscakluka/ema-core/core/audio"
)

type captureClient struct {
	audioContext *malgo.AllocatedContext
	device       *malgo.Device
	config       malgo.DeviceConfig

	onAudio func(audio []byte) // TODO: Consider a situation where there might be mutliple listeners

	mu sync.Mutex
}

func (c *captureClient) Init(audioContext *malgo.AllocatedContext) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	sampleRate := uint32(audio.DefaultSampleRate)
	channels := 1
	format := malgo.FormatS16
	bytesPerFrame := malgo.SampleSizeInBytes(format) * channels

	c.config = malgo.DefaultDeviceConfig(malgo.Capture)
	c.config.SampleRate = sampleRate
	c.config.Capture.Format = format
	c.config.Capture.Channels = uint32(channels)
	c.config.Alsa.NoMMap = 1
	c.config.PerformanceProfile = malgo.LowLatency
	c.config.PeriodSizeInFrames = 480
	c.config.Periods = 3

	c.audioContext = audioContext

	var err error
	c.device, err = malgo.InitDevice(c.audioContext.Context, c.config, malgo.DeviceCallbacks{
		Data: func(_, pInput []byte, frameCount uint32) {
			n := int(frameCount) * bytesPerFrame
			if len(pInput) < n || n == 0 {
				return
			}
			if c.onAudio != nil {
				c.onAudio(pInput[:n])
			}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to initialize capture device: %w", err)
	}

	return nil
}

func (c *captureClient) Start(onAudio func(audio []byte)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.device == nil {
		return fmt.Errorf("device not initialized")
	} else if c.device.IsStarted() {
		return nil
	}

	if err := c.device.Start(); err != nil {
		return fmt.Errorf("failed to start capture device: %w", err)
	}

	c.onAudio = onAudio
	return nil
}

func (c *captureClient) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.device == nil {
		return fmt.Errorf("device not initialized")
	} else if !c.device.IsStarted() {
		return nil
	}

	if err := c.device.Stop(); err != nil {
		return fmt.Errorf("failed to stop device: %w", err)
	}

	c.onAudio = nil
	return nil
}

func (c *captureClient) Uninit() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.device != nil {
		c.device.Uninit()
		c.device = nil
	}

	c.onAudio = nil
	return nil
}
