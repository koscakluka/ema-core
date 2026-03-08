package elevenlabs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
)

func (c *TextToSpeechClient) resolveAPIKey() (string, error) {
	if c != nil && strings.TrimSpace(c.apiKey) != "" {
		return c.apiKey, nil
	}

	apiKey := strings.TrimSpace(os.Getenv(envVarAPIKeyName))
	if apiKey == "" {
		return "", fmt.Errorf("%s is required", envVarAPIKeyName)
	}

	return apiKey, nil
}

func resolveDefaultVoiceIDFromEnv() string {
	voiceID := strings.TrimSpace(os.Getenv(envVarDefaultVoiceIDName))
	return voiceID
}

func websocketToHTTPURL(base string) (*url.URL, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}

	switch parsed.Scheme {
	case "wss":
		parsed.Scheme = "https"
	case "ws":
		parsed.Scheme = "http"
	case "https", "http":
	default:
		return nil, fmt.Errorf("unsupported base url scheme %q", parsed.Scheme)
	}

	return parsed, nil
}

func readResponseBody(body io.ReadCloser) string {
	if body == nil {
		return ""
	}

	bytes, err := io.ReadAll(io.LimitReader(body, 4096))
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(bytes))
}

func (c *TextToSpeechClient) connectWebsocket(ctx context.Context, encoding encodingInfo, apiKey string) (*websocket.Conn, error) {
	if c == nil {
		return nil, fmt.Errorf("elevenlabs client is nil")
	}

	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base url: %w", err)
	}

	basePath := strings.TrimSuffix(baseURL.Path, "/")
	baseURL.Path = basePath + "/v1/text-to-speech/" + url.PathEscape(c.VoiceID()) + "/stream-input"

	query := baseURL.Query()
	query.Set("output_format", encoding.OutputFormat)
	query.Set("enable_logging", strconv.FormatBool(c.enableLogging))
	query.Set("enable_ssml_parsing", strconv.FormatBool(c.enableSSMLParsing))
	query.Set("inactivity_timeout", strconv.Itoa(c.inactivityTimeoutSeconds))
	query.Set("sync_alignment", strconv.FormatBool(c.syncAlignment))
	if c.modelID != "" {
		query.Set("model_id", c.modelID)
	}

	// TODO: Support `apply_text_normalization`, `auto_mode`, and `seed`
	// query parameters as stable client options.
	baseURL.RawQuery = query.Encode()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, baseURL.String(), http.Header{"xi-api-key": {apiKey}})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to elevenlabs websocket: %w", err)
	}

	return conn, nil
}
