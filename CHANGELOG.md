# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic
Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `core/WithInterruptionHandlerV2` option and `core/InterruptionHandlerV2`
  interface to pass a context-aware interruption handler
- `core/texttospeech/deepgram.TextToSpeechClient.SetVoice` method
- `core/texttospeech/deepgram.TextToSpeechClient.Restart` method for resetting
  a streaming session

### Changed

- **Breaking:** module path renamed to `github.com/koscakluka/ema-core` (all
  imports updated)
- `core/Orchestrator.Orchestrate` now accepts a base context used across
  agent/tool calls and interruption handling
- `core/llms.Stream.Chunks` now requires a `context.Context` for streaming
  iteration
- `core/Orchestrator` now runs turn processing through the active-turn pipeline
  (`core/activeTurn`), affecting how speaking, pausing, and cancellation
  propagate to audio/text buffers

### Deprecated

- `core/ClassifyWithContext` option (since v0.0.14) for interruption
  classification in favor of interruption handlers

### Removed

### Fixed

- `core/audio/miniaudio` playback processing to reduce artifacts and mark
  handling issues
- `core/texttospeech/deepgram.TextToSpeechClient` restart behavior to preserve
  post-restart text buffering

### Security

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

[unreleased]: https://github.com/koscakluka/ema-core/compare/v0.0.13...HEAD
[v0.0.13]: https://github.com/koscakluka/ema-core/compare/v0.0.12...v0.0.13
[v0.0.12]: https://github.com/koscakluka/ema-core/compare/v0.0.11...v0.0.12
