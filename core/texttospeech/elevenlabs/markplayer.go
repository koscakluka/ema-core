package elevenlabs

import (
	"encoding/base64"
	"fmt"
	"iter"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/koscakluka/ema-core/core/audio"
)

type marker struct {
	marks         []playerMark
	marksPlayhead int
	audioPlayhead time.Duration

	mu sync.Mutex
}

func (p *marker) AddText(text string) {
	p.init()

	p.mu.Lock()
	defer p.mu.Unlock()
	p.marks[len(p.marks)-1].text.WriteString(text)
}

func (p *marker) NewMark() {
	p.init()

	p.mu.Lock()
	defer p.mu.Unlock()
	p.marks = append(p.marks, playerMark{})
}

func (p *marker) Finalise() {
	p.init()

	p.mu.Lock()
	defer p.mu.Unlock()
	if lastMarkIdx := len(p.marks) - 1; lastMarkIdx >= 0 && p.marks[lastMarkIdx].text.Len() == 0 {
		p.marks[lastMarkIdx].final = true
		return
	}

	p.marks = append(p.marks, playerMark{final: true})
}

// Play emits audio and mark elements for a single receiveMessage.
//
// Important interruption semantics:
//   - Returning false from yield stops emission immediately for the current call.
//   - No additional elements from the same message are emitted after interruption.
//   - Internal marker progression still advances for the consumed receiveMessage,
//     so pending marks in that message may be skipped and never emitted later.
//   - A subsequent Play call continues from the advanced internal state and emits
//     elements only for what remains after the skipped progression.
func (p *marker) Play(msg receiveMessage, encoding audio.EncodingInfo) iter.Seq2[markerElement, error] {
	p.init()

	audio, err := base64.StdEncoding.DecodeString(msg.Audio)
	if err != nil {
		return func(yield func(markerElement, error) bool) {
			yield(markerElement{}, fmt.Errorf("failed to decode audio from base64: %w", err))
			return
		}
	}

	return func(yield func(markerElement, error) bool) {
		p.mu.Lock()
		defer p.mu.Unlock()

		if msg.Alignment != nil {
			marksPlayhead := 0
			for i, mark := range p.marks {
				marksPlayhead = i
				if mark.playhead < mark.text.Len() {
					break
				}
			}

			for i := range len(msg.Alignment.Chars) {
				if len(p.marks) <= marksPlayhead {
					break
				}

				p.marks[marksPlayhead].playhead += 1

				if p.marks[marksPlayhead].playhead >= p.marks[marksPlayhead].text.Len() {
					p.marks[marksPlayhead].endMs = time.Duration(msg.Alignment.CharStartTimesMs[i]+msg.Alignment.CharDurationsMs[i]) * time.Millisecond
					marksPlayhead++
				}
			}
		}

		audioPlayhead := 0
		broken := false

		for audioPlayhead < len(audio) {
			leftoverAudioDuration := samplesDuration(len(audio[audioPlayhead:]), encoding)

			if len(p.marks) <= p.marksPlayhead ||
				p.marks[p.marksPlayhead].final ||
				!p.marks[p.marksPlayhead].hasEnd() ||
				p.audioPlayhead+leftoverAudioDuration < p.marks[p.marksPlayhead].endMs {
				p.audioPlayhead += leftoverAudioDuration
				if !broken {
					yield(markerElement{Type: markerElementTypeAudio, Audio: audio[audioPlayhead:]}, nil)
				}
				return
			}

			audioToPlayDuration := p.marks[p.marksPlayhead].endMs - p.audioPlayhead
			markedAudioSamples := audioSamples(audioToPlayDuration, encoding)
			if audioPlayhead+markedAudioSamples > len(audio) {
				// TODO: Check if this ever happens, it shouldn't technically
				log.Println("audio playhead exceeds audio length, clamping")
				markedAudioSamples = len(audio) - audioPlayhead
			}

			mark := p.marks[p.marksPlayhead]
			p.marksPlayhead++
			audioStart := audioPlayhead
			audioPlayhead += markedAudioSamples
			p.audioPlayhead += audioToPlayDuration

			if !broken {
				if !yield(markerElement{Type: markerElementTypeAudio, Audio: audio[audioStart : audioStart+markedAudioSamples]}, nil) {
					broken = true
				}
			}

			if !broken {
				if !yield(markerElement{Type: markerElementTypeMark, Mark: mark.text.String()}, nil) {
					broken = true
				}
			}
		}
	}
}

func (p *marker) FlushPendingMarks() []string {
	p.init()

	p.mu.Lock()
	defer p.mu.Unlock()

	marks := make([]string, 0)
	for p.marksPlayhead < len(p.marks) {
		mark := p.marks[p.marksPlayhead]
		if mark.final {
			break
		}

		p.marksPlayhead++
		marks = append(marks, mark.text.String())
	}

	return marks
}

func (p *marker) init() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.marks) > 0 {
		return
	}

	p.marks = append(p.marks, playerMark{})
}

type markerElement struct {
	Audio []byte
	Mark  string
	Type  markerElementType
}

type markerElementType string

const (
	markerElementTypeAudio markerElementType = "audio"
	markerElementTypeMark  markerElementType = "mark"
)

type playerMark struct {
	text strings.Builder

	playhead int
	endMs    time.Duration

	// final indicates if the mark is the final mark
	final bool
}

func (p *playerMark) hasEnd() bool {
	return p.text.Len() == p.playhead
}
