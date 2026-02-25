// Package protocol defines JSON protocol messages for communication between Gateway and Openexec.
package protocol

import (
	"encoding/json"
	"errors"
	"time"
)

// Audio stream message type constants.
const (
	// TypeAudioStreamStart is the type identifier for audio stream start messages.
	TypeAudioStreamStart = "audio_stream_start"

	// TypeAudioStreamStartAck is the type identifier for audio stream start acknowledgement.
	TypeAudioStreamStartAck = "audio_stream_start_ack"

	// TypeAudioStreamChunk is the type identifier for audio data chunk messages.
	TypeAudioStreamChunk = "audio_stream_chunk"

	// TypeAudioStreamEnd is the type identifier for audio stream end messages.
	TypeAudioStreamEnd = "audio_stream_end"

	// TypeAudioStreamEndAck is the type identifier for audio stream end acknowledgement.
	TypeAudioStreamEndAck = "audio_stream_end_ack"
)

// Errors specific to audio stream handling.
var (
	ErrMissingStreamID     = errors.New("message missing stream_id field")
	ErrInvalidAudioFormat  = errors.New("invalid audio format")
	ErrInvalidChunkData    = errors.New("invalid chunk data")
	ErrInvalidSequence     = errors.New("invalid sequence number")
	ErrStreamAlreadyExists = errors.New("stream already exists")
	ErrStreamNotFound      = errors.New("stream not found")
)

// AudioFormat represents supported audio formats for streaming.
type AudioFormat string

const (
	// AudioFormatPCM16 is raw PCM 16-bit audio.
	AudioFormatPCM16 AudioFormat = "pcm16"

	// AudioFormatOpus is Opus encoded audio.
	AudioFormatOpus AudioFormat = "opus"

	// AudioFormatMP3 is MP3 encoded audio.
	AudioFormatMP3 AudioFormat = "mp3"

	// AudioFormatOGG is OGG Vorbis encoded audio.
	AudioFormatOGG AudioFormat = "ogg"

	// AudioFormatWAV is WAV encoded audio.
	AudioFormatWAV AudioFormat = "wav"

	// AudioFormatWebM is WebM encoded audio.
	AudioFormatWebM AudioFormat = "webm"
)

// AudioStreamStart represents a request to initiate an audio stream.
// It contains metadata about the audio format and stream configuration.
type AudioStreamStart struct {
	BaseMessage

	// StreamID is a unique identifier for this audio stream.
	StreamID string `json:"stream_id"`

	// Format is the audio encoding format (e.g., "pcm16", "opus", "mp3").
	Format AudioFormat `json:"format"`

	// SampleRate is the audio sample rate in Hz (e.g., 16000, 44100, 48000).
	SampleRate int `json:"sample_rate"`

	// Channels is the number of audio channels (1 for mono, 2 for stereo).
	Channels int `json:"channels"`

	// BitDepth is the bits per sample (typically 16 or 24 for PCM).
	BitDepth int `json:"bit_depth,omitempty"`

	// ExpectedDurationMs is the expected total duration in milliseconds (optional).
	ExpectedDurationMs int64 `json:"expected_duration_ms,omitempty"`

	// SourceType indicates the origin of the audio (e.g., "voice_message", "audio_file", "live").
	SourceType string `json:"source_type,omitempty"`

	// Metadata contains additional key-value pairs for custom data.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// AudioStreamStartAck acknowledges the start of an audio stream.
type AudioStreamStartAck struct {
	BaseMessage

	// StreamID is the unique identifier for this audio stream.
	StreamID string `json:"stream_id"`

	// Accepted indicates whether the stream was accepted.
	Accepted bool `json:"accepted"`

	// MaxChunkSize is the maximum allowed chunk size in bytes (optional hint).
	MaxChunkSize int `json:"max_chunk_size,omitempty"`

	// Error contains error details if the stream was rejected.
	Error string `json:"error,omitempty"`
}

// AudioStreamChunk represents a chunk of audio data in the stream.
type AudioStreamChunk struct {
	BaseMessage

	// StreamID is the unique identifier for this audio stream.
	StreamID string `json:"stream_id"`

	// Sequence is the monotonically increasing sequence number for ordering.
	Sequence int64 `json:"sequence"`

	// Data is the base64-encoded audio data for this chunk.
	Data string `json:"data"`

	// DurationMs is the duration of audio in this chunk in milliseconds.
	DurationMs int `json:"duration_ms,omitempty"`

	// IsFinal indicates this is the last chunk in the stream (alternative to AudioStreamEnd).
	IsFinal bool `json:"is_final,omitempty"`
}

// AudioStreamEnd signals the end of an audio stream.
type AudioStreamEnd struct {
	BaseMessage

	// StreamID is the unique identifier for this audio stream.
	StreamID string `json:"stream_id"`

	// TotalChunks is the total number of chunks sent in this stream.
	TotalChunks int64 `json:"total_chunks"`

	// TotalBytes is the total number of audio bytes sent.
	TotalBytes int64 `json:"total_bytes"`

	// TotalDurationMs is the total duration of audio in milliseconds.
	TotalDurationMs int64 `json:"total_duration_ms,omitempty"`

	// Checksum is an optional checksum of all audio data (e.g., MD5 or SHA256 hex).
	Checksum string `json:"checksum,omitempty"`

	// Cancelled indicates the stream was cancelled before completion.
	Cancelled bool `json:"cancelled,omitempty"`

	// CancelReason provides the reason if the stream was cancelled.
	CancelReason string `json:"cancel_reason,omitempty"`
}

// AudioStreamEndAck acknowledges the end of an audio stream.
type AudioStreamEndAck struct {
	BaseMessage

	// StreamID is the unique identifier for this audio stream.
	StreamID string `json:"stream_id"`

	// Success indicates whether the stream was processed successfully.
	Success bool `json:"success"`

	// ChunksReceived is the number of chunks actually received.
	ChunksReceived int64 `json:"chunks_received"`

	// BytesReceived is the total bytes received.
	BytesReceived int64 `json:"bytes_received"`

	// ProcessingResult contains any result data from processing the audio.
	ProcessingResult map[string]interface{} `json:"processing_result,omitempty"`

	// Error contains error details if processing failed.
	Error string `json:"error,omitempty"`
}

// NewAudioStreamStart creates a new audio stream start message.
func NewAudioStreamStart(requestID, streamID string, format AudioFormat, sampleRate, channels int) *AudioStreamStart {
	return &AudioStreamStart{
		BaseMessage: BaseMessage{
			Type:      TypeAudioStreamStart,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		StreamID:   streamID,
		Format:     format,
		SampleRate: sampleRate,
		Channels:   channels,
	}
}

// NewAudioStreamStartWithOptions creates a new audio stream start with additional options.
func NewAudioStreamStartWithOptions(requestID, streamID string, format AudioFormat, sampleRate, channels, bitDepth int, sourceType string) *AudioStreamStart {
	msg := NewAudioStreamStart(requestID, streamID, format, sampleRate, channels)
	msg.BitDepth = bitDepth
	msg.SourceType = sourceType
	return msg
}

// NewAudioStreamStartAck creates a new audio stream start acknowledgement.
func NewAudioStreamStartAck(requestID, streamID string, accepted bool) *AudioStreamStartAck {
	return &AudioStreamStartAck{
		BaseMessage: BaseMessage{
			Type:      TypeAudioStreamStartAck,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		StreamID: streamID,
		Accepted: accepted,
	}
}

// NewAudioStreamStartAckError creates a new audio stream start rejection.
func NewAudioStreamStartAckError(requestID, streamID, errorMsg string) *AudioStreamStartAck {
	return &AudioStreamStartAck{
		BaseMessage: BaseMessage{
			Type:      TypeAudioStreamStartAck,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		StreamID: streamID,
		Accepted: false,
		Error:    errorMsg,
	}
}

// NewAudioStreamChunk creates a new audio stream chunk message.
func NewAudioStreamChunk(requestID, streamID string, sequence int64, data string) *AudioStreamChunk {
	return &AudioStreamChunk{
		BaseMessage: BaseMessage{
			Type:      TypeAudioStreamChunk,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		StreamID: streamID,
		Sequence: sequence,
		Data:     data,
	}
}

// NewAudioStreamChunkFinal creates a final audio stream chunk message.
func NewAudioStreamChunkFinal(requestID, streamID string, sequence int64, data string) *AudioStreamChunk {
	chunk := NewAudioStreamChunk(requestID, streamID, sequence, data)
	chunk.IsFinal = true
	return chunk
}

// NewAudioStreamEnd creates a new audio stream end message.
func NewAudioStreamEnd(requestID, streamID string, totalChunks, totalBytes int64) *AudioStreamEnd {
	return &AudioStreamEnd{
		BaseMessage: BaseMessage{
			Type:      TypeAudioStreamEnd,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		StreamID:    streamID,
		TotalChunks: totalChunks,
		TotalBytes:  totalBytes,
	}
}

// NewAudioStreamEndCancelled creates a cancelled audio stream end message.
func NewAudioStreamEndCancelled(requestID, streamID, reason string) *AudioStreamEnd {
	return &AudioStreamEnd{
		BaseMessage: BaseMessage{
			Type:      TypeAudioStreamEnd,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		StreamID:     streamID,
		Cancelled:    true,
		CancelReason: reason,
	}
}

// NewAudioStreamEndAck creates a new audio stream end acknowledgement.
func NewAudioStreamEndAck(requestID, streamID string, success bool, chunksReceived, bytesReceived int64) *AudioStreamEndAck {
	return &AudioStreamEndAck{
		BaseMessage: BaseMessage{
			Type:      TypeAudioStreamEndAck,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		StreamID:       streamID,
		Success:        success,
		ChunksReceived: chunksReceived,
		BytesReceived:  bytesReceived,
	}
}

// NewAudioStreamEndAckError creates a new audio stream end acknowledgement with error.
func NewAudioStreamEndAckError(requestID, streamID, errorMsg string, chunksReceived, bytesReceived int64) *AudioStreamEndAck {
	return &AudioStreamEndAck{
		BaseMessage: BaseMessage{
			Type:      TypeAudioStreamEndAck,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		StreamID:       streamID,
		Success:        false,
		ChunksReceived: chunksReceived,
		BytesReceived:  bytesReceived,
		Error:          errorMsg,
	}
}

// Validate validates the audio stream start message.
func (m *AudioStreamStart) Validate() error {
	if m.Type == "" {
		return ErrMissingType
	}
	if m.Type != TypeAudioStreamStart {
		return ErrUnknownType
	}
	if m.RequestID == "" {
		return ErrMissingRequestID
	}
	if m.StreamID == "" {
		return ErrMissingStreamID
	}
	if m.Format == "" {
		return ErrInvalidAudioFormat
	}
	// Validate format is one of the known values
	switch m.Format {
	case AudioFormatPCM16, AudioFormatOpus, AudioFormatMP3, AudioFormatOGG, AudioFormatWAV, AudioFormatWebM:
		// Valid format
	default:
		return ErrInvalidAudioFormat
	}
	// Validate sample rate (common rates: 8000, 16000, 22050, 44100, 48000)
	if m.SampleRate <= 0 {
		return errors.New("sample rate must be positive")
	}
	// Validate channels
	if m.Channels <= 0 || m.Channels > 8 {
		return errors.New("channels must be between 1 and 8")
	}
	return nil
}

// Validate validates the audio stream start acknowledgement.
func (m *AudioStreamStartAck) Validate() error {
	if m.Type == "" {
		return ErrMissingType
	}
	if m.Type != TypeAudioStreamStartAck {
		return ErrUnknownType
	}
	if m.RequestID == "" {
		return ErrMissingRequestID
	}
	if m.StreamID == "" {
		return ErrMissingStreamID
	}
	// If not accepted, error should be provided
	if !m.Accepted && m.Error == "" {
		return errors.New("error message required when stream not accepted")
	}
	return nil
}

// Validate validates the audio stream chunk message.
func (m *AudioStreamChunk) Validate() error {
	if m.Type == "" {
		return ErrMissingType
	}
	if m.Type != TypeAudioStreamChunk {
		return ErrUnknownType
	}
	if m.RequestID == "" {
		return ErrMissingRequestID
	}
	if m.StreamID == "" {
		return ErrMissingStreamID
	}
	if m.Sequence < 0 {
		return ErrInvalidSequence
	}
	if m.Data == "" {
		return ErrInvalidChunkData
	}
	return nil
}

// Validate validates the audio stream end message.
func (m *AudioStreamEnd) Validate() error {
	if m.Type == "" {
		return ErrMissingType
	}
	if m.Type != TypeAudioStreamEnd {
		return ErrUnknownType
	}
	if m.RequestID == "" {
		return ErrMissingRequestID
	}
	if m.StreamID == "" {
		return ErrMissingStreamID
	}
	// If cancelled, reason should be provided
	if m.Cancelled && m.CancelReason == "" {
		return errors.New("cancel reason required when stream is cancelled")
	}
	// If not cancelled, totals should be non-negative
	if !m.Cancelled {
		if m.TotalChunks < 0 {
			return errors.New("total chunks must be non-negative")
		}
		if m.TotalBytes < 0 {
			return errors.New("total bytes must be non-negative")
		}
	}
	return nil
}

// Validate validates the audio stream end acknowledgement.
func (m *AudioStreamEndAck) Validate() error {
	if m.Type == "" {
		return ErrMissingType
	}
	if m.Type != TypeAudioStreamEndAck {
		return ErrUnknownType
	}
	if m.RequestID == "" {
		return ErrMissingRequestID
	}
	if m.StreamID == "" {
		return ErrMissingStreamID
	}
	// If not successful, error should be provided
	if !m.Success && m.Error == "" {
		return errors.New("error message required when stream not successful")
	}
	return nil
}

// MarshalJSON implements json.Marshaler for AudioStreamStart.
func (m *AudioStreamStart) MarshalJSON() ([]byte, error) {
	type Alias AudioStreamStart
	return json.Marshal((*Alias)(m))
}

// UnmarshalJSON implements json.Unmarshaler for AudioStreamStart.
func (m *AudioStreamStart) UnmarshalJSON(data []byte) error {
	type Alias AudioStreamStart
	aux := (*Alias)(m)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}

// MarshalJSON implements json.Marshaler for AudioStreamStartAck.
func (m *AudioStreamStartAck) MarshalJSON() ([]byte, error) {
	type Alias AudioStreamStartAck
	return json.Marshal((*Alias)(m))
}

// UnmarshalJSON implements json.Unmarshaler for AudioStreamStartAck.
func (m *AudioStreamStartAck) UnmarshalJSON(data []byte) error {
	type Alias AudioStreamStartAck
	aux := (*Alias)(m)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}

// MarshalJSON implements json.Marshaler for AudioStreamChunk.
func (m *AudioStreamChunk) MarshalJSON() ([]byte, error) {
	type Alias AudioStreamChunk
	return json.Marshal((*Alias)(m))
}

// UnmarshalJSON implements json.Unmarshaler for AudioStreamChunk.
func (m *AudioStreamChunk) UnmarshalJSON(data []byte) error {
	type Alias AudioStreamChunk
	aux := (*Alias)(m)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}

// MarshalJSON implements json.Marshaler for AudioStreamEnd.
func (m *AudioStreamEnd) MarshalJSON() ([]byte, error) {
	type Alias AudioStreamEnd
	return json.Marshal((*Alias)(m))
}

// UnmarshalJSON implements json.Unmarshaler for AudioStreamEnd.
func (m *AudioStreamEnd) UnmarshalJSON(data []byte) error {
	type Alias AudioStreamEnd
	aux := (*Alias)(m)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}

// MarshalJSON implements json.Marshaler for AudioStreamEndAck.
func (m *AudioStreamEndAck) MarshalJSON() ([]byte, error) {
	type Alias AudioStreamEndAck
	return json.Marshal((*Alias)(m))
}

// UnmarshalJSON implements json.Unmarshaler for AudioStreamEndAck.
func (m *AudioStreamEndAck) UnmarshalJSON(data []byte) error {
	type Alias AudioStreamEndAck
	aux := (*Alias)(m)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}

// IsValidAudioFormat returns true if the given format is a valid audio format.
func IsValidAudioFormat(format AudioFormat) bool {
	switch format {
	case AudioFormatPCM16, AudioFormatOpus, AudioFormatMP3, AudioFormatOGG, AudioFormatWAV, AudioFormatWebM:
		return true
	default:
		return false
	}
}

// GetAudioFormatMimeType returns the MIME type for a given audio format.
func GetAudioFormatMimeType(format AudioFormat) string {
	switch format {
	case AudioFormatPCM16:
		return "audio/pcm"
	case AudioFormatOpus:
		return "audio/opus"
	case AudioFormatMP3:
		return "audio/mpeg"
	case AudioFormatOGG:
		return "audio/ogg"
	case AudioFormatWAV:
		return "audio/wav"
	case AudioFormatWebM:
		return "audio/webm"
	default:
		return "application/octet-stream"
	}
}
