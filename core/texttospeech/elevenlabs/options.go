package elevenlabs

type ClientOption func(*ClientOptions)

type VoiceType string

const (
	VoiceTypePersonal   VoiceType = "personal"
	VoiceTypeCommunity  VoiceType = "community"
	VoiceTypeDefault    VoiceType = "default"
	VoiceTypeWorkspace  VoiceType = "workspace"
	VoiceTypeNonDefault VoiceType = "non-default"
	VoiceTypeSaved      VoiceType = "saved"
)

// ClientOptions controls ElevenLabs websocket TTS behavior.
type ClientOptions struct {
	// APIKey overrides ELEVENLABS_API_KEY for this client.
	APIKey string
	// ModelID selects the ElevenLabs model.
	ModelID string
	// VoiceType is used as a default filter when searching voices.
	VoiceType VoiceType
	// BaseURL selects the websocket host/region.
	//
	// Defaults to wss://api.elevenlabs.io.
	BaseURL string
	// EnableLogging controls provider-side request logging.
	EnableLogging bool
	// InactivityTimeoutSeconds configures websocket inactivity timeout.
	InactivityTimeoutSeconds int
	// SyncAlignment enables alignment information in websocket responses.
	SyncAlignment bool
	// EnableSSMLParsing enables provider-side SSML parsing.
	EnableSSMLParsing bool

	// VoiceSettings are applied during the initialization message.
	VoiceSettings *RealtimeVoiceSettings
	// GenerationConfig controls chunk schedule behavior.
	GenerationConfig *GenerationConfig

	// TODO: Support single-use token authentication.
	// TODO: Support bearer authorization query/header auth.
	// TODO: Support pronunciation_dictionary_locators.
	// TODO: Support apply_text_normalization and auto_mode query options.
	// TODO: Support deterministic generation seed.
}

type RealtimeVoiceSettings struct {
	Stability       float64 `json:"stability,omitempty"`
	SimilarityBoost float64 `json:"similarity_boost,omitempty"`
	Style           float64 `json:"style,omitempty"`
	UseSpeakerBoost *bool   `json:"use_speaker_boost,omitempty"`
	Speed           float64 `json:"speed,omitempty"`
}

type GenerationConfig struct {
	ChunkLengthSchedule []int `json:"chunk_length_schedule,omitempty"`
}

func WithAPIKey(apiKey string) ClientOption {
	return func(options *ClientOptions) { options.APIKey = apiKey }
}

func WithModelID(modelID string) ClientOption {
	return func(options *ClientOptions) { options.ModelID = modelID }
}

func WithVoiceType(voiceType VoiceType) ClientOption {
	return func(options *ClientOptions) { options.VoiceType = voiceType }
}

func WithBaseURL(baseURL string) ClientOption {
	return func(options *ClientOptions) { options.BaseURL = baseURL }
}

func WithEnableLogging(enable bool) ClientOption {
	return func(options *ClientOptions) { options.EnableLogging = enable }
}

func WithInactivityTimeoutSeconds(timeout int) ClientOption {
	return func(options *ClientOptions) { options.InactivityTimeoutSeconds = timeout }
}

func WithSyncAlignment(enable bool) ClientOption {
	return func(options *ClientOptions) { options.SyncAlignment = enable }
}

func WithEnableSSMLParsing(enable bool) ClientOption {
	return func(options *ClientOptions) { options.EnableSSMLParsing = enable }
}

func WithVoiceSettings(settings RealtimeVoiceSettings) ClientOption {
	return func(options *ClientOptions) {
		copied := settings
		options.VoiceSettings = &copied
	}
}

func WithGenerationConfig(config GenerationConfig) ClientOption {
	return func(options *ClientOptions) {
		copied := config
		options.GenerationConfig = &copied
	}
}
