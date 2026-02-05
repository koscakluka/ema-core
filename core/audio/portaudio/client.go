package portaudio

import (
	"bytes"
	"context"
	"encoding/binary"
	"log"

	"github.com/gordonklaus/portaudio"
	"github.com/koscakluka/ema-core/core/audio"
)

type Client struct {
	bufferSize    int
	stream        *portaudio.Stream
	leftoverAudio []byte

	in  []int16
	out []int16
}

func NewClient(bufferSize int) (*Client, error) {
	err := portaudio.Initialize()
	if err != nil {
		log.Fatalf("Failed to initialize PortAudio: %v", err)
		return nil, err
	}

	in := make([]int16, bufferSize)
	out := make([]int16, bufferSize)
	stream, err := portaudio.OpenDefaultStream(1, 1, audio.DefaultSampleRate, bufferSize, in, out)
	if err != nil {
		log.Fatalf("Failed to open PortAudio stream: %v", err)
	}

	return &Client{
		bufferSize: bufferSize,
		stream:     stream,
		in:         in,
		out:        out,
	}, nil
}

func (c *Client) Stream(ctx context.Context, onAudio func(audio []byte)) error {
	log.Println("Starting microphone capture. Speak now...")
	if err := c.stream.Start(); err != nil {
		log.Fatalf("Failed to start PortAudio stream: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if err := c.stream.Read(); err != nil {
				log.Printf("Failed to read from PortAudio stream: %v", err)
			}

			audioBuffer := bytes.Buffer{}
			binary.Write(&audioBuffer, binary.LittleEndian, c.in)
			onAudio(audioBuffer.Bytes())
		}
	}
}

func (c *Client) Close() {
	c.stream.Close()
	portaudio.Terminate()
}

func (c *Client) SendAudio(audio []byte) error {
	bufferSize := c.bufferSize * 2

	// PERF: This is just to test this, there is no reason we should
	// kill performance by copying here
	audio = append(c.leftoverAudio, audio...)
	for i := range len(audio)/bufferSize + 1 {
		if (i+1)*bufferSize > len(audio) {
			c.leftoverAudio = make([]byte, len(audio)-i*bufferSize)
			copy(c.leftoverAudio, audio[i*bufferSize:])
			break
		}

		binary.Read(bytes.NewBuffer(audio[i*bufferSize:(i+1)*bufferSize]), binary.LittleEndian, c.out)
		c.stream.Write()
	}

	return nil
}

func (c *Client) ClearBuffer() {
	c.leftoverAudio = make([]byte, 0)
}

func (c *Client) AwaitMark() error {
	bufferSize := c.bufferSize * 2

	// PERF: This is just to test this, there is no reason we should
	// kill performance by copying here
	audio := c.leftoverAudio
	for i := range len(audio)/bufferSize + 1 {
		if (i+1)*bufferSize > len(audio) {
			c.leftoverAudio = make([]byte, 0)
			copy(c.leftoverAudio, audio[i*bufferSize:])
			break
		}

		binary.Read(bytes.NewBuffer(audio[i*bufferSize:(i+1)*bufferSize]), binary.LittleEndian, c.out)
		c.stream.Write()
	}
	return nil
}

func (c *Client) EncodingInfo() audio.EncodingInfo {
	return audio.EncodingInfo{
		SampleRate: audio.DefaultSampleRate,
		Format:     audio.EncodingLinear16,
	}
}
