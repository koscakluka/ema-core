# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic
Versioning](https://semver.org/spec/v2.0.0.html).

> Legend: **Breaking** marks changes that require consumer updates.

## [Unreleased]

## [v0.0.19] - 2026-02-24

### Added

- orchestration now emits lifecycle events for model responses, tool calls, and
  turn state changes so receivers can track progress end-to-end
- playback updates now include audio deltas and terminal legacy marks so
  spoken-text progress remains complete and compatible with older mark handling

### Changed

- **Breaking:** legacy orchestration input `events` were renamed to `triggers`
  (`core/triggers`, `llms.TriggerV0`, `TurnV1.Trigger`,
  `Orchestrator.HandleTrigger`, `WithTriggerHandlerV0`/`TriggerHandlerV0`, and
  `core/triggers/interruptions`); `core/events` was reintroduced as a separate
  receiver-facing emitted event contract
- **Breaking:** renamed orchestration event taxonomy to receiver-facing groups
  and kinds (`user_input.*`, `assistant_response.*`, `assistant_speech.*`,
  `assistant_playback.*`, `turn_state.*`), including event types and
  constructors in `core/events`
- **Breaking:** speaking-state events are now split into explicit
  `UserSpeechStarted`/`UserSpeechEnded` events

### Fixed

- playback transcript segment events are now append-only; regressions no longer
  emit replacement segments

## [v0.0.18] - 2026-02-23

### Added

- `core/WithSpokenTextCallback` and `core/WithSpokenTextDeltaCallback` for
  subscribing to playback-aligned spoken-text updates during `Orchestrate`
- `core/WithPartialInterimTranscriptionCallback` and
  `core/WithPartialTranscriptionCallback` for subscribing to partial STT updates
  during `Orchestrate`

### Changed

- spoken-text callbacks now continue updating during playback between confirmed
  marks with best-effort in-flight progress estimation
- pausing and resuming speech now keeps playback progress and spoken-text
  updates better aligned with what was heard

### Fixed

- speech output now waits for a confirmed TTS connection before playback starts
  to avoid dropped or misordered speech
- speech marks now include trailing spoken segments, and unknown or duplicate
  audio marks are ignored to keep spoken-text progress stable
- Deepgram STT callback dispatch now normalizes unset callbacks to no-op
  handlers and consistently emits configured interim/final/speech-state
  callbacks without scattered nil checks

## [v0.0.17] - 2026-02-21

### Added

- `core/EventHandlerV0` and `core/WithEventHandlerV0` for plugging custom event
  handlers into `core/Orchestrator`
- `core/conversations.ActiveContextV0` to expose live conversation history,
  active turn, and available tools to event handlers
- `core/ConversationV1` as a point-in-time conversation view returned by
  `core/Orchestrator.ConversationV1`
- `core/events/interruptions` event handlers (`NewEventHandlerWithStructuredPrompt`
  and `NewEventHandlerWithGeneralPrompt`) for integrated event processing and
  interruption classification
- interruption lifecycle event types in `core/events` for recording,
  resolving, and canceling turns during interruption handling

### Deprecated

- `core/WithInterruptionHandlerV0`, `core/WithInterruptionHandlerV1`, and
  `core/WithInterruptionHandlerV2` in favor of `core/WithEventHandlerV0`
- `core/Config` and `core/WithConfig`; use the capturing-audio controls
  (`RequestToCaptureAudio`, `StopRequestingToCaptureAudio`,
  `EnableAlwaysCapturingAudio`, `DisableAlwaysCapturingAudio`, and
  `IsAlwaysCapturingAudio`) instead
- `core/Orchestrator.StartRecording`, `StopRecording`,
  `EnableAlwaysRecording`, `DisableAlwaysRecording`, `IsAlwaysRecording`, and
  `SetAlwaysRecording` in favor of the capturing-audio controls above
- `core/Orchestrator.IsRecording` and `core/Orchestrator.IsSpeaking` fields in
  favor of `core/Orchestrator.IsCapturingAudio` and
  `core/Orchestrator.IsMuted`
- `core/Orchestrator.SetSpeaking` in favor of `core/Orchestrator.Mute` and
  `core/Orchestrator.Unmute`
- `core/Orchestrator.CallTool`; interruption-driven tool execution now runs
  through the event-handler pipeline
- legacy `TurnsV0`/`ConversationV0` mutation APIs and deprecated option aliases
  are compatibility-only shims
- legacy mutable conversation methods `Pop`, `Clear`, `Values`, and `RValues`
  on `core/context/ConversationV0` are compatibility-only shims in favor of
  `core/conversations.ActiveContextV0`

### Changed

- interruption handling now runs through the event-handler pipeline, so custom
  handlers can influence turn cancellation, continuation prompts, and tool calls
- **Breaking:** speaking/transcription orchestrate callbacks are emitted from
  live speech runtime callbacks; manually injected events via
  `Orchestrator.Handle` do not invoke these callback hooks
- prompts queued before `Orchestrate` starts are kept and processed once
  runtime is active
- turn execution dependencies are fixed at turn start, so client/config updates
  apply to subsequent turns instead of altering in-flight behavior

### Fixed

- prompt, stream, tool, and interruption-classifier failures now propagate as
  explicit errors instead of being silently ignored
- shutdown is now idempotent and queue-safe under concurrent activity, with
  active turns canceled cleanly
- startup failures no longer terminate the process via fatal exits and are
  surfaced as recoverable errors
- cancel-turn events now bypass interruption classification in built-in
  handlers, so cancellation requests are not swallowed while a turn is active
- OpenAI and Groq LLM clients now close HTTP response bodies in all code paths,
  and OpenAI history after tool calls is preserved
- Deepgram TTS invalid encoding errors now preserve the underlying cause
- typed-nil audio input/output clients are treated as unconfigured, preventing
  invalid runtime state and nil dispatch
- `SetSpeaking(true)` no longer stops an active turn's speech pipeline

## [v0.0.16] - 2026-02-16

### Added

- `core/events` package with typed orchestration event types for prompts,
  speech, transcriptions, and tool calls
- `core/Orchestrator.Handle` method for submitting custom orchestration events
- `core/WithInputAudioCallback` orchestrate option for observing raw input
  audio chunks

### Changed

- Speech and transcription processing now flow through event handling, so voice
  and text interruptions follow the same path as direct prompt/tool events
- Prompt/tool history serialization for Groq and OpenAI now reads from
  `TurnV1.Event`

### Deprecated

- `core/Orchestrator.QueuePrompt` in favor of `core/Orchestrator.SendPrompt`

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
- `core/Orchestrator.Turns` in favor of `core/Orchestrator.ConversationV0`
- legacy `core/context/TurnsV0` mutation helpers (`Push`, `Pop`, `Clear`,
  `Values`, `RValues`) are compatibility shims; prefer
  `core/context/ConversationV0` accessors where possible

### Removed

- **Breaking:** legacy interruption classifier APIs and types
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
- **Breaking:** `core/Orchestrator.Orchestrate` now accepts a base context used
  across agent/tool calls and interruption handling
- **Breaking:** `core/llms.Stream.Chunks` now requires a `context.Context` for
  streaming iteration

### Deprecated

- `core/ClassifyWithContext` option for interruption classification in favor of
  interruption handlers
- legacy TTS callback aliases `core/texttospeech.WithAudioCallback` and
  `core/texttospeech.WithAudioEndedCallback`, plus
  `core/texttospeech.TextToSpeechOptions.AudioCallback` and
  `core/texttospeech.TextToSpeechOptions.AudioEnded`, in favor of
  `WithSpeechAudioCallback`/`SpeechAudioCallback` and
  `WithSpeechMarkCallback`/`SpeechMarkCallback`
- Deepgram streaming methods `OpenStream`, `SendText`, `FlushBuffer`,
  `ClearBuffer`, and `CloseStream` in favor of
  `TextToSpeechClient.NewSpeechGeneratorV0` with `SpeechGeneratorV0`

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
  interface for `core/Orchestrator` with accompanying logic to use it instead
  of the built-in interruption handling mechanisms
- `core/WithInterruptionHandlerV1` option and `core/InterruptionHandlerV1`
  interface
- `core/WithAudioOutputV0` option and `core/AudioOutputV0` interface as a
  replacement for `core/WithAudioOutput` option and `core/AudioOutput`
  interface
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
- `core/llms/Turn.Stage` field, `core/llms/TurnStage` type and accompanying
  enum
- `core/llms/Turn.Interruptions` field for storing interruptions that happened
  in a particular turn
- `core/audio/miniaudio/playbackClient.Mark` method

### Changed

- `core/Orchestrator` uses `core/llms/Turn` instead of `core/llms/Message` for
  internal storage of history
- `core/Orchestrator.callTool` into a public method (`CallTool`)
- **Breaking:** `core/Orchestrator.Turns` returns `core/context/TurnsV0`
  interface instead of `core/Turns` struct
- clients inside `core/llms` package now use their own message types based off
  of their APIs requirements, converted from passed `core/llms/Turns`
- **Breaking:** `core/llms/Turn` uses `core/llms/TurnRoles` type instead of
  `core/llms/MessageRole` diverging from the `core/llms/Message` type
- **Breaking:** `core/llms/ToolCall` uses a flat structure instead of one with
  a nested `Function` field
- `core/llms/WithSystemPrompt` saves the prompt in `Instructions` field in
  addition to using the first message in the deprecated `Messages` field, to
  allow the prompt to be used with `Turns` field
- `core/llms/ToMessages` and `core/llms/ToTurns` functions convert between
  `Message` slice and `Turn` slice considering the new `TurnRoles` type

### Deprecated

- `core/Orchestrator.Cancel` method
- `core/Orchestrator.Messages` method
- `core/WithAudioOutput` option and `core/AudioOutput` interface in favor of
  `core/WithAudioOutputV0` to allow for reintroduction of `WithAudioOutput`
  when stable
- `core/WithLLM` option
- `core/LLMWithPrompt` interface in favor of `core/LLMWithGeneralPrompt`
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

### Added

- `core/llms/openai` prompt and streaming clients for GPT-4o, GPT-4.1, and
  GPT-5 Nano, including tool-call support
- turn-first conversation APIs via `core/llms.Turn`, `core/llms.WithTurns`,
  and `core/Orchestrator.Turns`
- orchestration LLM extension points via `core/WithStreamingLLM`,
  `core/LLMWithGeneralPrompt`, `core/LLMWithPrompt`,
  `core/ClassifierWithInterruptionLLM`, and
  `core/ClassifierWithGeneralPromptLLM`

### Changed

- **Breaking:** `core/WithLLM` now accepts `LLMWithPrompt`; stream-only clients
  should use `core/WithStreamingLLM`
- prompt options now carry both `Messages` and `Turns`; legacy message options
  are mapped for compatibility
- stream usage reporting renamed and expanded token/time fields
  (`InputTokens`/`OutputTokens` and detail structs) with compatibility aliases
  retained

### Deprecated

- `core/llms.Message` as the primary conversation unit; prefer
  `core/llms.Turn` (message type remains as a compatibility alias)

## [v0.0.11] - 2025-10-31

### Added

- `core/Config` and `core/WithConfig` for orchestrator runtime configuration
- `core/AudioInputFine` interface and orchestration controls for fine-grained
  capture start/stop
- `core/Orchestrator.Messages` and `core/Orchestrator.IsAlwaysRecording` for
  runtime state inspection

### Changed

- **Breaking:** `core/Orchestrator.AlwaysRecording` is now config-backed;
  consumers should use `IsAlwaysRecording`/`SetAlwaysRecording`
- miniaudio client internals were split into dedicated capture and playback
  components for safer lifecycle handling

## [v0.0.10] - 2025-10-08

### Added

- streaming orchestration support via `core/LLMWithStream` and
  `PromptWithStream`
- `core/llms` streaming abstractions (`Stream`, stream chunk interfaces, and
  usage payloads)
- structured prompting support via `core/llms.StructuredPromptOption` and
  `core/InterruptionLLM`
- dedicated Groq model constructors and model capability cards in
  `core/llms/groq`

### Changed

- interruption classification now supports structured-output classifiers when
  the configured LLM provides schema prompting
- prompt option handling was split into general, streaming, and structured
  option paths

## [v0.0.9] - 2025-10-01

### Added

- interactive text input support in the CLI app (`main` package)

### Changed

- **Breaking:** `core/Orchestrator.SendPrompt` no longer accepts
  `OrchestrateOption` arguments and now reuses active orchestrate callbacks

### Fixed

- miniaudio playback device now starts on init so the first playback does not
  get dropped

## [v0.0.8] - 2025-10-01

### Added

- `core/Orchestrator.SendPrompt` method for pushing direct prompts
- `core/WithAudioCallback` and `core/WithAudioEndedCallback` orchestrate
  options

### Changed

- orchestration startup now treats speech/audio components as optional and only
  wires streams that are configured

### Fixed

- orchestration now guards nil component combinations to avoid runtime panics

## [v0.0.7] - 2025-09-29

### Changed

- a default `SimpleInterruptionClassifier` is now installed automatically when
  no interruption classifier is configured

## [v0.0.6] - 2025-09-29

### Added

- interruption classifier extension points:
  `core/InterruptionClassifier`, `core/WithInterruptionClassifier`,
  `core/SimpleInterruptionClassifier`, `core/ClassifierWithTools`, and
  `core/ClassifyWithTools`

### Fixed

- Groq prompt responses now return only response messages (not the full request
  thread), preventing duplicated history in callers

## [v0.0.5] - 2025-09-26

### Added

- Groq client constructor options: `WithModel`, `WithTools`, `WithSystemPrompt`,
  and `WithAPIKey`

### Changed

- **Breaking:** `groq.NewClient` now accepts options and returns
  `(*Client, error)` instead of returning `*Client` with implicit nil failure

## [v0.0.4] - 2025-09-26

### Added

- `core/WithLLM` orchestrator option and `core/LLM` interface for injecting LLM
  implementations
- shared `core/llms` prompt option helpers (`WithSystemPrompt`, `WithMessages`,
  `WithTools`, `WithForcedTools`, `WithStream`)

### Changed

- **Breaking:** orchestration prompting now depends on an injected LLM; the
  internal default Groq client path was removed

### Fixed

- playback now flushes leftover audio correctly while awaiting marks

## [v0.0.3] - 2025-09-25

### Added

- tool-call aware message fields in `core/llms.Message` (`ToolCalls`,
  `ToolCallID`) and `core/llms.ToolCall`
- orchestration tool configuration via `core/WithTools` and
  `core/WithOrchestrationTools`
- option-based orchestration callback API:
  `core/OrchestrateOption` and callback helpers

### Changed

- **Breaking:** callback-struct based speech loop (`ListenForSpeech`) was
  replaced with `core/Orchestrator.Orchestrate(...OrchestrateOption)`
- orchestration tools now use shared `core/llms.Tool` types instead of Groq
  package-specific types

### Fixed

- Groq prompting now returns the full assistant/tool response sequence needed by
  orchestrator history handling

## [v0.0.2] - 2025-09-25

### Added

- audio encoding metadata support via `audio.EncodingInfo` and encoding-aware
  options for speech-to-text and text-to-speech clients

## [v0.0.1] - 2025-09-25

### Added

- playback mark-awaiting support for miniaudio output synchronization

[unreleased]: https://github.com/koscakluka/ema-core/compare/v0.0.19...HEAD
[v0.0.19]: https://github.com/koscakluka/ema-core/compare/v0.0.18...v0.0.19
[v0.0.18]: https://github.com/koscakluka/ema-core/compare/v0.0.17...v0.0.18
[v0.0.17]: https://github.com/koscakluka/ema-core/compare/v0.0.16...v0.0.17
[v0.0.16]: https://github.com/koscakluka/ema-core/compare/v0.0.15...v0.0.16
[v0.0.15]: https://github.com/koscakluka/ema-core/compare/v0.0.14...v0.0.15
[v0.0.14]: https://github.com/koscakluka/ema-core/compare/v0.0.13...v0.0.14
[v0.0.13]: https://github.com/koscakluka/ema-core/compare/v0.0.12...v0.0.13
[v0.0.12]: https://github.com/koscakluka/ema-core/compare/v0.0.11...v0.0.12
[v0.0.11]: https://github.com/koscakluka/ema-core/compare/v0.0.10...v0.0.11
[v0.0.10]: https://github.com/koscakluka/ema-core/compare/v0.0.9...v0.0.10
[v0.0.9]: https://github.com/koscakluka/ema-core/compare/v0.0.8...v0.0.9
[v0.0.8]: https://github.com/koscakluka/ema-core/compare/v0.0.7...v0.0.8
[v0.0.7]: https://github.com/koscakluka/ema-core/compare/v0.0.6...v0.0.7
[v0.0.6]: https://github.com/koscakluka/ema-core/compare/v0.0.5...v0.0.6
[v0.0.5]: https://github.com/koscakluka/ema-core/compare/v0.0.4...v0.0.5
[v0.0.4]: https://github.com/koscakluka/ema-core/compare/v0.0.3...v0.0.4
[v0.0.3]: https://github.com/koscakluka/ema-core/compare/v0.0.2...v0.0.3
[v0.0.2]: https://github.com/koscakluka/ema-core/compare/v0.0.1...v0.0.2
[v0.0.1]: https://github.com/koscakluka/ema-core/releases/tag/v0.0.1
