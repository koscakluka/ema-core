# Soniox STT client

This package provides a real-time Soniox speech-to-text client for EMA.

## Features

- WebSocket streaming to Soniox real-time STT
- Endpoint-based utterance finalization (`<end>`/`<fin>` handling)
- Short synthetic-silence streaming after input pauses to help endpoint
  detection settle before keepalive-only mode
- Keepalive control messages during audio pauses

## Configuration

The client reads `SONIOX_API_KEY` by default.

Optional constructor options:

- `WithAPIKey(string)`

### Defaults

- Model: `ModelSTTRTV4`
- Endpoint detection: enabled
- Max endpoint delay: `1000ms`
- Keepalive silence threshold: `10s`

TODO: expose model/endpoint/keepalive tuning options once the public Soniox API
surface is finalized.

### What each option does

- `WithAPIKey`
  - Use this to override `SONIOX_API_KEY` for one client instance.

## Callback mapping

The Soniox implementation maps token stream updates into EMA callbacks like this:

- Non-final tokens (`is_final=false`)
  - `WithPartialInterimTranscriptionCallback`
  - `WithInterimTranscriptionCallback`
- Final tokens (`is_final=true`, excluding markers)
  - `WithPartialTranscriptionCallback`
- Finalization markers (`<end>` or `<fin>`) or `finished=true`
  - `WithTranscriptionCallback`
  - `WithSpeechEndedCallback`
- TODO: Language callback mapping is intentionally disabled until the public
  language API is finalized.

## Usage

```go
import (
    "context"

    orchestration "github.com/koscakluka/ema-core/core"
    "github.com/koscakluka/ema-core/core/speechtotext/soniox"
)

func configure(ctx context.Context) *orchestration.Orchestrator {
    stt := soniox.NewClient(ctx)

    return orchestration.NewOrchestrator(
        orchestration.WithSpeechToTextClient(stt),
    )
}
```

## Notes

- Audio format is derived from EMA encoding settings and sent as Soniox raw
  format (`pcm_s16le`, `alaw`, `mulaw`).
- Mono is assumed (`num_channels=1`) because EMA input is currently mono.
