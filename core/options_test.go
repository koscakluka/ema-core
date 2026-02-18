package orchestration

import "testing"

func TestWithConfigForwardsAlwaysRecording(t *testing.T) {
	o := NewOrchestrator(WithConfig(&Config{AlwaysRecording: false}))

	if o.IsAlwaysRecording() {
		t.Fatalf("expected deprecated config option to disable always recording")
	}
}

func TestWithConfigNilIsNoop(t *testing.T) {
	o := NewOrchestrator(WithConfig(nil))

	if !o.IsAlwaysRecording() {
		t.Fatalf("expected nil config to keep default always recording")
	}
}
