# ElevenLabs TTS client

This package provides a real-time ElevenLabs text-to-speech client for EMA.

## Features

- WebSocket streaming to ElevenLabs single-context TTS endpoint
- Incremental text sending via `SpeechGeneratorV0.SendText`
- Mark bridging via `SpeechGeneratorV0.Mark` using alignment-aware progress
- End-of-text signaling via `SpeechGeneratorV0.EndOfText`
- Voice discovery via `SearchVoices`

## Configuration

The client reads `ELEVENLABS_API_KEY` by default.

Voice selection behavior:

- `voice_id` passed to `NewTextToSpeechClient` is used when non-empty
- if empty, `ELEVENLABS_DEFAULT_VOICE_ID` is used when set
- otherwise the client searches `voice_type=default` and uses the first voice in
  the returned list

Optional constructor options include:

- `WithAPIKey(string)`
- `WithModelID(string)`
- `WithVoiceType(VoiceType)`
- `WithBaseURL(string)`
- `WithEnableLogging(bool)`
- `WithInactivityTimeoutSeconds(int)`
- `WithSyncAlignment(bool)`
- `WithEnableSSMLParsing(bool)`
- `WithVoiceSettings(RealtimeVoiceSettings)`
- `WithGenerationConfig(GenerationConfig)`

## Usage

```go
import (
    "context"

    orchestration "github.com/koscakluka/ema-core/core"
    "github.com/koscakluka/ema-core/core/texttospeech/elevenlabs"
)

func configure(ctx context.Context) (*orchestration.Orchestrator, error) {
    tts, err := elevenlabs.NewTextToSpeechClient(ctx, "",
        elevenlabs.WithVoiceType(elevenlabs.VoiceTypeNonDefault),
    )
    if err != nil {
        return nil, err
    }

    voices, err := tts.SearchVoices(ctx, elevenlabs.SearchVoicesOptions{
        Search: "Rachel",
        PageSize: 5,
    })
    if err != nil {
        return nil, err
    }
    if len(voices.Voices) > 0 {
        if err := tts.SetVoiceID(voices.Voices[0].VoiceID); err != nil {
            return nil, err
        }
    }

    return orchestration.NewOrchestrator(
        orchestration.WithTextToSpeechClientV1(tts),
    ), nil
}
```

## TODOs

- support `pronunciation_dictionary_locators`
- support single-use token and bearer auth modes
- expose richer alignment metadata through public callback/report APIs
- support multi-context websocket endpoint
