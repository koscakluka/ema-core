# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic
Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `core/EventHandlerV0` and `core/WithEventHandlerV0` for plugging custom event
  handlers into `core/Orchestrator`
- internal orchestration control events in `core/events` for canceling turns and
  interruption lifecycle updates
- `core/events/interruptions` event handlers (`NewEventHandlerWithStructuredPrompt`
  and `NewEventHandlerWithGeneralPrompt`) that combine basic event processing
  and interruption classification

### Deprecated

- `core/WithInterruptionHandlerV0`, `core/WithInterruptionHandlerV1`, and
  `core/WithInterruptionHandlerV2` in favor of `core/WithEventHandlerV0`

### Changed

- default internal event handling now owns interruption processing and emits
  control events consumed by the orchestrator

### Fixed

- OpenAI and Groq LLM clients now close HTTP response bodies in all code paths,
  including per-request handling inside Groq prompt loops

## [v0.0.16] - 2026-02-16

### Added

- `core/events` package with typed orchestration event types for prompts, speech,
  transcriptions, and tool calls
- `core/Orchestrator.Handle` method for submitting custom orchestration events
- `core/WithInputAudioCallback` orchestrate option for observing raw input audio
  chunks

### Changed

- Speech and transcription processing now flow through event handling, so voice
  and text interruptions follow the same path as direct prompt/tool events
- Prompt/tool history serialization for Groq and OpenAI now reads from
  `TurnV1.Event`

## [v0.0.15] - 2026-02-07

### Added

- `core/llms/TurnV1` with IDs, triggers, and response tracking
- `core/context/ConversationV0` interface for turn history access
- LLM prompt options now accept `TurnV1` history
- OpenCode helper commands for the repo

### Changed

- **Breaking:** removed deprecated interruption classifier package paths
- Groq/OpenAI history serialization now uses `TurnV1` triggers and responses
- Default audio encoding is now 16kHz linear16 across audio I/O and Deepgram

### Deprecated

- `core/llms/Turn` in favor of `core/llms/TurnV1`
- `core/Orchestrator.Turns` in favor of `core/Orchestrator.Conversation`

### Removed

- legacy interruption classifier APIs and types
- leftover `main` package file

### Fixed

- encoding defaults now stay consistent across components
- audio timing and mark alignment now respect encoding byte size
- turn cancellation now clears buffers and cancels active TTS generation
- Deepgram keep-alive silence generation now matches selected encoding

## [v0.0.14] - 2026-02-04

### Added

- `core/WithInterruptionHandlerV2` option and `core/InterruptionHandlerV2`
  interface to pass a context-aware interruption handler
- `core/texttospeech/deepgram.TextToSpeechClient.SetVoice` method
- `core/texttospeech/deepgram.TextToSpeechClient.Restart` method for resetting
  a streaming session
- `core/texttospeech` SpeechGenerator interface with Deepgram implementation
- Support for TTSV1 client in orchestration
- OpenTelemetry instrumentation for orchestration, interruption handlers, and
  Groq LLMs

### Changed

- **Breaking:** module path renamed to `github.com/koscakluka/ema-core` (all
  imports updated)
- `core/Orchestrator.Orchestrate` now accepts a base context used across
  agent/tool calls and interruption handling
- `core/llms.Stream.Chunks` now requires a `context.Context` for streaming
  iteration

### Deprecated

- `core/ClassifyWithContext` option for interruption classification in favor of
  interruption handlers

### Fixed

- `core/audio/miniaudio` playback processing to reduce artifacts and mark
  handling issues
- `core/texttospeech/deepgram.TextToSpeechClient` restart behavior to preserve
  post-restart text buffering
- Speaking, pausing, and cancellation now propagate more consistently across the
  turn lifecycle
- Turns now end cleanly when no audio output is produced
- Speech processing is skipped when no TTS client is configured
- Deepgram streaming now signals speech ended on non-empty buffer and handles
  stream closure gracefully
- TTS client restarts per active turn as is expected by providers
- Groq Request to First Token timing now marks only after first chunk is
  received
- Audio buffer wakes on speaking state changes

## [v0.0.13] - 2025-11-20

### Added

- `core/WithInterruptionHandlerV0` option and `core/InterruptionHandlerV0`
  interface for `core/Orchestrator` with accompanying logic to use it instead of
  the built-in interruption handling mechanisms
- `core/WithInterruptionHandlerV1` option and `core/InterruptionHandlerV1`
  interface
- `core/WithAudioOutputV0` option and `core/AudioOutputV0` interface as a
  replacement for `core/WithAudioOutput` option and `core/AudioOutput` interface
- `core/WithAudioOutputV1` option and `core/AudioOutputV1` interface
- `core/Orchestrator.QueuePrompt` method
- `core/Orchestrator.CallToolWithPrompt` method
- `core/Orchestrator.CancelTurn` method as a replacement for `Cancel` method
- `core/Orchestrator.PauseTurn` method
- `core/Orchestrator.UnpauseTurn` method
- `core/context/TurnsV0` interface
- `core/interruptions/OrchestratorV0` interface
- `core/interruptions/llm` package with a simple interruption handler
- `core/llms/Response` struct as a future replacement for deprecated
  `core/llms/Message`
- `core/llms/InterruptionV0` struct
- `core/llms/ToolCall.Response` field to store the response of the tool call
  instead of having it as a separate message role (that is now deprecated)
- `core/llms/TurnRoles` type and accompanying enum
- `core/llms/Turn.Cancelled` field to replace the cancelled flag in
  `core/Orchestrator`
- `core/llms/Turn.Stage` field, `core/llms/TurnStage` type and accompanying enum
- `core/llms/Turn.Interruptions` field for storing interruptions that happened in
  a particular turn
- `core/audio/miniaudio/playbackClient.Mark` method

### Changed

- `core/Orchestrator` uses `core/llms/Turn` instead of `core/llms/Message` for
  internal storage of history
- `core/Orchestrator.callTool` into a public method (`CallTool`)
- `core/Orchestrator.Turns` to return `core/context/TurnsV0` interface instead
  of `core/Turns` struct
- clients inside `core/llms` package now use their own message types based off
  of their APIs requirements, converted from passed `core/llms/Turns`
- `core/llms/Turn` uses `core/llms/TurnRoles` type instead of
  `core/llms/MessageRole` diverging from the `core/llms/Message` type
- `core/llms/ToolCall` uses a flat structure instead of a one with a nested
  `Function` field
- `core/llms/WithSystemPrompt` saves the prompt in `Instructions` field in
  addition to using the first message in the deprecated `Messages` field, to allow
  the prompt to be used with `Turns` field
- `core/llms/ToMessages` and `core/llms/ToTurns` functions convert to between
  `Message` slice and `Turn` slice considering the new `TurnRoles` type

### Deprecated

- `core/Orchestrator.Cancel` method
- `core/Orchestrator.Messages` method
- `core/WithAudioOutput` option and `core/AudioOutput` interface in favor of
  `core/WithAudioOutputV0` to allow for reintroduction of `WithAudioOutput` when
  stable
- `core/WithLLM` option
- `core/InterruptionLLM` interface
- `core/WithInterruptionClassifier` option and `core/InterruptionClassifier`
  interface
- `core/SimpleInterruptionClassifier` struct
- `core/llms/Turn.ToolCallID` field
- `core/llms/MessageRole` type and accompanying enum
- `core/llms/ToolCallFunction` struct and field using it inside
  `core/llms/ToolCall`
- `core/llms/ToolCall.Type` field

### Fixed

- `core/texttospeech/deepgram/` panicking when concurrently calling `SendText`
  or `FlushBuffer`
- `core/texttospeech/deepgram/` dropping text sent after `FlushBuffer` call or
  returned confirmation

## [v0.0.12] - 2025-11-05

[unreleased]: https://github.com/koscakluka/ema-core/compare/v0.0.16...HEAD
[v0.0.16]: https://github.com/koscakluka/ema-core/compare/v0.0.15...v0.0.16
[v0.0.15]: https://github.com/koscakluka/ema-core/compare/v0.0.14...v0.0.15
[v0.0.14]: https://github.com/koscakluka/ema-core/compare/v0.0.13...v0.0.14
[v0.0.13]: https://github.com/koscakluka/ema-core/compare/v0.0.12...v0.0.13
[v0.0.12]: https://github.com/koscakluka/ema-core/compare/v0.0.11...v0.0.12
