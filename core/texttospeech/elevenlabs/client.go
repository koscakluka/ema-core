package elevenlabs

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

const (
	envVarAPIKeyName         = "ELEVENLABS_API_KEY"
	envVarDefaultVoiceIDName = "ELEVENLABS_DEFAULT_VOICE_ID"

	defaultVoiceSearchPageSize = 1

	defaultBaseURL                  = "wss://api.elevenlabs.io"
	defaultEnableLogging            = true
	defaultSyncAlignment            = true
	defaultInactivityTimeoutSeconds = 20
)

type TextToSpeechClient struct {
	mu sync.RWMutex

	apiKey string

	voiceID   string
	voiceType VoiceType
	modelID   string

	baseURL                  string
	enableLogging            bool
	inactivityTimeoutSeconds int
	syncAlignment            bool
	enableSSMLParsing        bool

	voiceSettings    *RealtimeVoiceSettings
	generationConfig *GenerationConfig
}

// NewTextToSpeechClient creates an ElevenLabs websocket TTS client.
func NewTextToSpeechClient(ctx context.Context, voiceID string, opts ...ClientOption) (*TextToSpeechClient, error) {
	voiceID = strings.TrimSpace(voiceID)

	options := ClientOptions{
		BaseURL:                  defaultBaseURL,
		EnableLogging:            defaultEnableLogging,
		InactivityTimeoutSeconds: defaultInactivityTimeoutSeconds,
		SyncAlignment:            defaultSyncAlignment,
	}
	for _, opt := range opts {
		opt(&options)
	}

	if strings.TrimSpace(options.BaseURL) == "" {
		options.BaseURL = defaultBaseURL
	}
	if options.InactivityTimeoutSeconds <= 0 {
		options.InactivityTimeoutSeconds = defaultInactivityTimeoutSeconds
	}

	client := &TextToSpeechClient{
		apiKey: options.APIKey,

		voiceID:   voiceID,
		voiceType: options.VoiceType,
		modelID:   options.ModelID,

		baseURL:                  options.BaseURL,
		enableLogging:            options.EnableLogging,
		inactivityTimeoutSeconds: options.InactivityTimeoutSeconds,
		syncAlignment:            options.SyncAlignment,
		enableSSMLParsing:        options.EnableSSMLParsing,

		voiceSettings:    options.VoiceSettings,
		generationConfig: options.GenerationConfig,
	}

	if client.voiceID != "" {
		return client, nil
	}

	envVoiceID := resolveDefaultVoiceIDFromEnv()
	if envVoiceID != "" {
		client.voiceID = envVoiceID
		return client, nil
	}

	resolvedVoiceID, err := client.resolveDefaultVoiceIDBySearch(ctx)
	if err != nil {
		return nil, err
	}
	client.voiceID = resolvedVoiceID

	return client, nil
}

func (c *TextToSpeechClient) SetVoiceID(voiceID string) error {
	voiceID = strings.TrimSpace(voiceID)
	if voiceID == "" {
		return fmt.Errorf("voice id is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.voiceID = voiceID

	return nil
}

func (c *TextToSpeechClient) VoiceID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.voiceID
}

func (c *TextToSpeechClient) SetVoiceType(voiceType VoiceType) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.voiceType = voiceType
}

func (c *TextToSpeechClient) VoiceType() VoiceType {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.voiceType
}

func (c *TextToSpeechClient) resolveDefaultVoiceIDBySearch(ctx context.Context) (string, error) {
	result, err := c.SearchVoices(ctx, SearchVoicesOptions{
		PageSize:  defaultVoiceSearchPageSize,
		VoiceType: VoiceTypeDefault,
	})
	if err != nil {
		return "", fmt.Errorf("failed to resolve default elevenlabs voice id via search: %w", err)
	}
	if len(result.Voices) == 0 {
		return "", fmt.Errorf("failed to resolve default elevenlabs voice id via search: no voices returned")
	}

	voiceID := strings.TrimSpace(result.Voices[0].VoiceID)
	if voiceID == "" {
		return "", fmt.Errorf("failed to resolve default elevenlabs voice id via search: empty voice id in first result")
	}

	return voiceID, nil
}
