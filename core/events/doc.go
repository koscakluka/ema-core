// Package events defines the typed orchestration event contract.
//
// Event kinds are grouped by receiver-facing namespaces:
//
//   - user_input.*
//   - assistant_response.*
//   - assistant_speech.*
//   - assistant_playback.*
//   - turn_state.*
//
// Semantics used across the package:
//
//   - Frame: binary audio frame/chunk payload.
//   - Segment: append-only text piece emitted in stream order.
//   - Updated: mutable point-in-time snapshot that can change over time.
//   - Final: terminal immutable text/state for the current stream/turn phase.
//   - Ended: lifecycle boundary indicating stream completion.
//
// user_input events
//
//   - UserAudioFrame (user_input.audio_frame): raw user input audio frame.
//   - UserSpeechStarted (user_input.speech_started): speech activity began.
//   - UserSpeechEnded (user_input.speech_ended): speech activity ended.
//   - UserTranscriptInterimSegmentUpdated (user_input.transcript_interim_segment_updated):
//     mutable interim tail segment update.
//   - UserTranscriptInterimUpdated (user_input.transcript_interim_updated):
//     mutable interim full transcript snapshot.
//   - UserTranscriptSegment (user_input.transcript_segment): finalized,
//     append-only transcript segment.
//   - UserTranscriptFinal (user_input.transcript_final): terminal full
//     transcript for the utterance.
//
// assistant_response events
//
//   - AssistantResponseSegment (assistant_response.segment): streamed response
//     text segment.
//   - AssistantResponseFinal (assistant_response.final): response text stream
//     is complete.
//
// assistant_speech events
//
//   - AssistantSpeechFrame (assistant_speech.frame): synthesized speech audio
//     frame.
//   - AssistantSpeechMarkGenerated (assistant_speech.mark_generated): TTS mark
//     generated with transcript text associated with that mark.
//   - AssistantSpeechFinal (assistant_speech.final): TTS generation ended.
//
// assistant_playback events
//
//   - AssistantPlaybackTranscriptUpdated (assistant_playback.transcript_updated):
//     mutable playback transcript snapshot.
//   - AssistantPlaybackTranscriptSegment (assistant_playback.transcript_segment):
//     append-only playback transcript segment.
//   - AssistantPlaybackEnded (assistant_playback.ended): playback ended for the
//     current response; includes final transcript snapshot known by the player.
//
// turn_state events
//
//   - TurnCancelled (turn_state.cancelled): current turn was cancelled.
package events
