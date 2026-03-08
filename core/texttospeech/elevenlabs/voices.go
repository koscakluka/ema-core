package elevenlabs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type SearchVoicesOptions struct {
	NextPageToken string
	PageSize      int
	Search        string
	Sort          string
	SortDirection string
	VoiceType     VoiceType
	Category      string
	CollectionID  string

	IncludeTotalCount *bool
	VoiceIDs          []string
}

type SearchVoicesResponse struct {
	Voices        []Voice `json:"voices"`
	HasMore       bool    `json:"has_more"`
	TotalCount    int     `json:"total_count"`
	NextPageToken string  `json:"next_page_token"`
}

type Voice struct {
	VoiceID           string             `json:"voice_id"`
	Name              string             `json:"name"`
	Category          string             `json:"category"`
	Description       string             `json:"description"`
	PreviewURL        string             `json:"preview_url"`
	Labels            map[string]string  `json:"labels"`
	VerifiedLanguages []VerifiedLanguage `json:"verified_languages"`
}

type VerifiedLanguage struct {
	Language string `json:"language"`
	Locale   string `json:"locale"`
}

func (c *TextToSpeechClient) SearchVoices(ctx context.Context, options SearchVoicesOptions) (SearchVoicesResponse, error) {
	if c == nil {
		return SearchVoicesResponse{}, fmt.Errorf("elevenlabs client is nil")
	}

	apiKey, err := c.resolveAPIKey()
	if err != nil {
		return SearchVoicesResponse{}, err
	}

	baseURL, err := websocketToHTTPURL(c.baseURL)
	if err != nil {
		return SearchVoicesResponse{}, err
	}

	basePath := strings.TrimSuffix(baseURL.Path, "/")
	baseURL.Path = basePath + "/v2/voices"

	voiceType := options.VoiceType
	if voiceType == "" {
		voiceType = c.VoiceType()
	}
	if voiceType != "" && !isValidVoiceType(voiceType) {
		return SearchVoicesResponse{}, fmt.Errorf("invalid voice type: %q", voiceType)
	}

	query := buildVoiceSearchQuery(options, voiceType)
	baseURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return SearchVoicesResponse{}, fmt.Errorf("failed to create voice search request: %w", err)
	}
	req.Header.Set("xi-api-key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return SearchVoicesResponse{}, fmt.Errorf("failed to search elevenlabs voices: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := readResponseBody(resp.Body)
		if body != "" {
			return SearchVoicesResponse{}, fmt.Errorf("elevenlabs voice search failed with status %d: %s", resp.StatusCode, body)
		}
		return SearchVoicesResponse{}, fmt.Errorf("elevenlabs voice search failed with status %d", resp.StatusCode)
	}

	result := SearchVoicesResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return SearchVoicesResponse{}, fmt.Errorf("failed to decode voice search response: %w", err)
	}

	return result, nil
}

func buildVoiceSearchQuery(options SearchVoicesOptions, voiceType VoiceType) url.Values {
	query := url.Values{}
	if options.NextPageToken != "" {
		query.Set("next_page_token", options.NextPageToken)
	}
	if options.PageSize > 0 {
		query.Set("page_size", strconv.Itoa(options.PageSize))
	}
	if options.Search != "" {
		query.Set("search", options.Search)
	}
	if options.Sort != "" {
		query.Set("sort", options.Sort)
	}
	if options.SortDirection != "" {
		query.Set("sort_direction", options.SortDirection)
	}
	if voiceType != "" {
		query.Set("voice_type", string(voiceType))
	}
	if options.Category != "" {
		query.Set("category", options.Category)
	}
	if options.CollectionID != "" {
		query.Set("collection_id", options.CollectionID)
	}
	if options.IncludeTotalCount != nil {
		query.Set("include_total_count", strconv.FormatBool(*options.IncludeTotalCount))
	}
	if len(options.VoiceIDs) > 0 {
		query.Set("voice_ids", strings.Join(options.VoiceIDs, ","))
	}

	return query
}

func isValidVoiceType(voiceType VoiceType) bool {
	switch voiceType {
	case VoiceTypePersonal, VoiceTypeCommunity, VoiceTypeDefault, VoiceTypeWorkspace, VoiceTypeNonDefault, VoiceTypeSaved:
		return true
	default:
		return false
	}
}
