package orchestration

import (
	"sync"
	"testing"

	"github.com/koscakluka/ema-core/core/audio"
)

func TestAudioOutputSnapshotKeepsOriginalClientAfterSet(t *testing.T) {
	original := &snapshotAudioOutputV0{}
	replacement := &snapshotAudioOutputV0{}

	facade := newAudioOutput(original)
	snapshot := facade.Snapshot()

	facade.Set(replacement)

	snapshot.SendAudio([]byte{0x01})
	snapshot.Clear()

	if got := original.sendCalls(); got != 1 {
		t.Fatalf("expected snapshot to send audio through original client once, got %d", got)
	}
	if got := original.clearCalls(); got != 1 {
		t.Fatalf("expected snapshot to clear original client once, got %d", got)
	}
	if got := replacement.sendCalls(); got != 0 {
		t.Fatalf("expected replacement to receive no snapshot audio, got %d", got)
	}
	if got := replacement.clearCalls(); got != 0 {
		t.Fatalf("expected replacement to receive no snapshot clears, got %d", got)
	}

	facade.SendAudio([]byte{0x02})
	facade.Clear()

	if got := replacement.sendCalls(); got != 1 {
		t.Fatalf("expected facade to send audio through replacement client once, got %d", got)
	}
	if got := replacement.clearCalls(); got != 1 {
		t.Fatalf("expected facade to clear replacement client once, got %d", got)
	}
}

func TestAudioOutputFacadeTreatsTypedNilAsUnconfigured(t *testing.T) {
	var outputClient *snapshotAudioOutputV0

	facade := newAudioOutput(outputClient)

	if facade.isConfigured() {
		t.Fatalf("expected typed nil output client to be treated as unconfigured")
	}
	if facade.base != nil {
		t.Fatalf("expected base client to be nil for typed nil output client")
	}
	if facade.v0 != nil || facade.v1 != nil {
		t.Fatalf("expected version-specific clients to be nil for typed nil output client")
	}

	callbackCalled := false
	facade.Mark("typed-nil-mark", func(string) {
		callbackCalled = true
	})
	if !callbackCalled {
		t.Fatalf("expected unconfigured facade to invoke mark callback")
	}
}

func TestAudioOutputFacadeSetTypedNilClearsConfiguration(t *testing.T) {
	facade := newAudioOutput(&snapshotAudioOutputV0{})
	if !facade.isConfigured() {
		t.Fatalf("expected facade to start configured")
	}

	var outputClient *snapshotAudioOutputV0
	facade.Set(outputClient)

	if facade.isConfigured() {
		t.Fatalf("expected facade to become unconfigured after setting typed nil output client")
	}
	if facade.base != nil {
		t.Fatalf("expected base client to be nil after setting typed nil output client")
	}
	if facade.v0 != nil || facade.v1 != nil {
		t.Fatalf("expected version-specific clients to be nil after setting typed nil output client")
	}
}

func TestAudioOutputSnapshotPreservesMarkBehaviorAcrossSet(t *testing.T) {
	original := &snapshotAudioOutputV1{}
	replacement := &snapshotAudioOutputV0{}

	facade := newAudioOutput(original)
	snapshot := facade.Snapshot()

	facade.Set(replacement)

	callbackCalls := 0
	snapshot.Mark("snapshot-mark", func(string) {
		callbackCalls++
	})

	if got := callbackCalls; got != 1 {
		t.Fatalf("expected snapshot mark callback once, got %d", got)
	}
	if got := original.markCalls(); got != 1 {
		t.Fatalf("expected snapshot to use original v1 mark handler once, got %d", got)
	}
	if got := replacement.awaitCalls(); got != 0 {
		t.Fatalf("expected replacement v0 await mark to remain unused, got %d", got)
	}

	if snapshot.supportsCallbackMarks != true {
		t.Fatalf("expected snapshot to preserve callback mark support")
	}
	if facade.supportsCallbackMarks != false {
		t.Fatalf("expected facade to reconfigure callback mark support after Set")
	}
}

type snapshotAudioOutputV0 struct {
	mu         sync.Mutex
	sendCount  int
	clearCount int
	awaitCount int
}

func (output *snapshotAudioOutputV0) EncodingInfo() audio.EncodingInfo {
	return audio.GetDefaultEncodingInfo()
}

func (output *snapshotAudioOutputV0) SendAudio([]byte) error {
	output.mu.Lock()
	output.sendCount++
	output.mu.Unlock()
	return nil
}

func (output *snapshotAudioOutputV0) ClearBuffer() {
	output.mu.Lock()
	output.clearCount++
	output.mu.Unlock()
}

func (output *snapshotAudioOutputV0) AwaitMark() error {
	output.mu.Lock()
	output.awaitCount++
	output.mu.Unlock()
	return nil
}

func (output *snapshotAudioOutputV0) sendCalls() int {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.sendCount
}

func (output *snapshotAudioOutputV0) clearCalls() int {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.clearCount
}

func (output *snapshotAudioOutputV0) awaitCalls() int {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.awaitCount
}

type snapshotAudioOutputV1 struct {
	mu         sync.Mutex
	sendCount  int
	clearCount int
	markCount  int
}

func (output *snapshotAudioOutputV1) EncodingInfo() audio.EncodingInfo {
	return audio.GetDefaultEncodingInfo()
}

func (output *snapshotAudioOutputV1) SendAudio([]byte) error {
	output.mu.Lock()
	output.sendCount++
	output.mu.Unlock()
	return nil
}

func (output *snapshotAudioOutputV1) ClearBuffer() {
	output.mu.Lock()
	output.clearCount++
	output.mu.Unlock()
}

func (output *snapshotAudioOutputV1) Mark(mark string, callback func(string)) error {
	output.mu.Lock()
	output.markCount++
	output.mu.Unlock()
	callback(mark)
	return nil
}

func (output *snapshotAudioOutputV1) markCalls() int {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.markCount
}
