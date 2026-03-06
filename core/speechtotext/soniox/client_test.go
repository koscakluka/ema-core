package soniox

import (
	"context"
	"testing"
)

func TestNewClientDefaults(t *testing.T) {
	client := NewClient(context.Background())

	if client.apiKey != "" {
		t.Fatalf("expected empty api key by default")
	}
}

func TestNewClientWithAPIKeyOption(t *testing.T) {
	client := NewClient(context.Background(), WithAPIKey("test-key"))

	if client.apiKey != "test-key" {
		t.Fatalf("expected api key to be set from option")
	}
}
