// Package soniox provides a real-time Soniox speech-to-text client.
//
// The client implements EMA's generic speech-to-text interface and can be
// plugged into orchestration via core.WithSpeechToTextClient.
//
// High-level flow:
//   - Transcribe opens a WebSocket connection and sends Soniox start config.
//   - SendAudio streams raw audio frames as binary messages.
//   - Incoming token responses are converted into EMA callbacks/events.
//   - Keepalive control messages are sent during audio pauses.
//   - Close sends an empty frame to finish the stream gracefully.
package soniox
