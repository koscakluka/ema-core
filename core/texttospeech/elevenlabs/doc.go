// Package elevenlabs provides an ElevenLabs websocket text-to-speech client.
//
// The client implements EMA's SpeechGeneratorV0 interface and can be plugged
// into orchestration via core.WithTextToSpeechClientV1.
//
// Current implementation targets the single-context websocket endpoint:
//   - wss://api.elevenlabs.io/v1/text-to-speech/{voice_id}/stream-input
//
// TODO: Add support for the multi-context websocket endpoint when the
// orchestration layer exposes context routing requirements.
package elevenlabs
