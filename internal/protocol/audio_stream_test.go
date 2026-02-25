package protocol

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestNewAudioStreamStart(t *testing.T) {
	msg := NewAudioStreamStart("req-123", "stream-456", AudioFormatOpus, 48000, 2)

	if msg.Type != TypeAudioStreamStart {
		t.Errorf("Type = %v, want %v", msg.Type, TypeAudioStreamStart)
	}
	if msg.RequestID != "req-123" {
		t.Errorf("RequestID = %v, want %v", msg.RequestID, "req-123")
	}
	if msg.StreamID != "stream-456" {
		t.Errorf("StreamID = %v, want %v", msg.StreamID, "stream-456")
	}
	if msg.Format != AudioFormatOpus {
		t.Errorf("Format = %v, want %v", msg.Format, AudioFormatOpus)
	}
	if msg.SampleRate != 48000 {
		t.Errorf("SampleRate = %v, want %v", msg.SampleRate, 48000)
	}
	if msg.Channels != 2 {
		t.Errorf("Channels = %v, want %v", msg.Channels, 2)
	}
	if msg.Timestamp == "" {
		t.Error("Timestamp should not be empty")
	}
}

func TestNewAudioStreamStartWithOptions(t *testing.T) {
	msg := NewAudioStreamStartWithOptions("req-123", "stream-456", AudioFormatPCM16, 16000, 1, 16, "voice_message")

	if msg.BitDepth != 16 {
		t.Errorf("BitDepth = %v, want %v", msg.BitDepth, 16)
	}
	if msg.SourceType != "voice_message" {
		t.Errorf("SourceType = %v, want %v", msg.SourceType, "voice_message")
	}
}

func TestAudioStreamStart_Validate(t *testing.T) {
	tests := []struct {
		name    string
		msg     *AudioStreamStart
		wantErr error
	}{
		{
			name:    "valid message",
			msg:     NewAudioStreamStart("req-123", "stream-456", AudioFormatOpus, 48000, 2),
			wantErr: nil,
		},
		{
			name: "missing type",
			msg: &AudioStreamStart{
				BaseMessage: BaseMessage{RequestID: "req-123"},
				StreamID:    "stream-456",
				Format:      AudioFormatOpus,
				SampleRate:  48000,
				Channels:    2,
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			msg: &AudioStreamStart{
				BaseMessage: BaseMessage{Type: "wrong_type", RequestID: "req-123"},
				StreamID:    "stream-456",
				Format:      AudioFormatOpus,
				SampleRate:  48000,
				Channels:    2,
			},
			wantErr: ErrUnknownType,
		},
		{
			name: "missing request ID",
			msg: &AudioStreamStart{
				BaseMessage: BaseMessage{Type: TypeAudioStreamStart},
				StreamID:    "stream-456",
				Format:      AudioFormatOpus,
				SampleRate:  48000,
				Channels:    2,
			},
			wantErr: ErrMissingRequestID,
		},
		{
			name: "missing stream ID",
			msg: &AudioStreamStart{
				BaseMessage: BaseMessage{Type: TypeAudioStreamStart, RequestID: "req-123"},
				Format:      AudioFormatOpus,
				SampleRate:  48000,
				Channels:    2,
			},
			wantErr: ErrMissingStreamID,
		},
		{
			name: "empty format",
			msg: &AudioStreamStart{
				BaseMessage: BaseMessage{Type: TypeAudioStreamStart, RequestID: "req-123"},
				StreamID:    "stream-456",
				Format:      "",
				SampleRate:  48000,
				Channels:    2,
			},
			wantErr: ErrInvalidAudioFormat,
		},
		{
			name: "invalid format",
			msg: &AudioStreamStart{
				BaseMessage: BaseMessage{Type: TypeAudioStreamStart, RequestID: "req-123"},
				StreamID:    "stream-456",
				Format:      "invalid_format",
				SampleRate:  48000,
				Channels:    2,
			},
			wantErr: ErrInvalidAudioFormat,
		},
		{
			name: "zero sample rate",
			msg: &AudioStreamStart{
				BaseMessage: BaseMessage{Type: TypeAudioStreamStart, RequestID: "req-123"},
				StreamID:    "stream-456",
				Format:      AudioFormatOpus,
				SampleRate:  0,
				Channels:    2,
			},
			wantErr: nil, // Will fail with custom error
		},
		{
			name: "zero channels",
			msg: &AudioStreamStart{
				BaseMessage: BaseMessage{Type: TypeAudioStreamStart, RequestID: "req-123"},
				StreamID:    "stream-456",
				Format:      AudioFormatOpus,
				SampleRate:  48000,
				Channels:    0,
			},
			wantErr: nil, // Will fail with custom error
		},
		{
			name: "too many channels",
			msg: &AudioStreamStart{
				BaseMessage: BaseMessage{Type: TypeAudioStreamStart, RequestID: "req-123"},
				StreamID:    "stream-456",
				Format:      AudioFormatOpus,
				SampleRate:  48000,
				Channels:    10,
			},
			wantErr: nil, // Will fail with custom error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				}
			} else if tt.name == "zero sample rate" || tt.name == "zero channels" || tt.name == "too many channels" {
				if err == nil {
					t.Error("Validate() expected error, got nil")
				}
			}
		})
	}
}

func TestNewAudioStreamStartAck(t *testing.T) {
	msg := NewAudioStreamStartAck("req-123", "stream-456", true)

	if msg.Type != TypeAudioStreamStartAck {
		t.Errorf("Type = %v, want %v", msg.Type, TypeAudioStreamStartAck)
	}
	if !msg.Accepted {
		t.Error("Accepted should be true")
	}
	if msg.Error != "" {
		t.Errorf("Error should be empty, got %v", msg.Error)
	}
}

func TestNewAudioStreamStartAckError(t *testing.T) {
	msg := NewAudioStreamStartAckError("req-123", "stream-456", "unsupported format")

	if msg.Accepted {
		t.Error("Accepted should be false")
	}
	if msg.Error != "unsupported format" {
		t.Errorf("Error = %v, want %v", msg.Error, "unsupported format")
	}
}

func TestAudioStreamStartAck_Validate(t *testing.T) {
	tests := []struct {
		name    string
		msg     *AudioStreamStartAck
		wantErr bool
	}{
		{
			name:    "valid accepted",
			msg:     NewAudioStreamStartAck("req-123", "stream-456", true),
			wantErr: false,
		},
		{
			name:    "valid rejected with error",
			msg:     NewAudioStreamStartAckError("req-123", "stream-456", "reason"),
			wantErr: false,
		},
		{
			name: "rejected without error",
			msg: &AudioStreamStartAck{
				BaseMessage: BaseMessage{Type: TypeAudioStreamStartAck, RequestID: "req-123"},
				StreamID:    "stream-456",
				Accepted:    false,
				Error:       "",
			},
			wantErr: true,
		},
		{
			name: "missing stream ID",
			msg: &AudioStreamStartAck{
				BaseMessage: BaseMessage{Type: TypeAudioStreamStartAck, RequestID: "req-123"},
				Accepted:    true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewAudioStreamChunk(t *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte("audio data"))
	msg := NewAudioStreamChunk("req-123", "stream-456", 0, data)

	if msg.Type != TypeAudioStreamChunk {
		t.Errorf("Type = %v, want %v", msg.Type, TypeAudioStreamChunk)
	}
	if msg.Sequence != 0 {
		t.Errorf("Sequence = %v, want %v", msg.Sequence, 0)
	}
	if msg.Data != data {
		t.Errorf("Data = %v, want %v", msg.Data, data)
	}
	if msg.IsFinal {
		t.Error("IsFinal should be false")
	}
}

func TestNewAudioStreamChunkFinal(t *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte("final audio data"))
	msg := NewAudioStreamChunkFinal("req-123", "stream-456", 10, data)

	if !msg.IsFinal {
		t.Error("IsFinal should be true")
	}
	if msg.Sequence != 10 {
		t.Errorf("Sequence = %v, want %v", msg.Sequence, 10)
	}
}

func TestAudioStreamChunk_Validate(t *testing.T) {
	tests := []struct {
		name    string
		msg     *AudioStreamChunk
		wantErr error
	}{
		{
			name:    "valid chunk",
			msg:     NewAudioStreamChunk("req-123", "stream-456", 0, "YXVkaW8gZGF0YQ=="),
			wantErr: nil,
		},
		{
			name: "negative sequence",
			msg: &AudioStreamChunk{
				BaseMessage: BaseMessage{Type: TypeAudioStreamChunk, RequestID: "req-123"},
				StreamID:    "stream-456",
				Sequence:    -1,
				Data:        "YXVkaW8gZGF0YQ==",
			},
			wantErr: ErrInvalidSequence,
		},
		{
			name: "empty data",
			msg: &AudioStreamChunk{
				BaseMessage: BaseMessage{Type: TypeAudioStreamChunk, RequestID: "req-123"},
				StreamID:    "stream-456",
				Sequence:    0,
				Data:        "",
			},
			wantErr: ErrInvalidChunkData,
		},
		{
			name: "missing stream ID",
			msg: &AudioStreamChunk{
				BaseMessage: BaseMessage{Type: TypeAudioStreamChunk, RequestID: "req-123"},
				Sequence:    0,
				Data:        "YXVkaW8gZGF0YQ==",
			},
			wantErr: ErrMissingStreamID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewAudioStreamEnd(t *testing.T) {
	msg := NewAudioStreamEnd("req-123", "stream-456", 100, 512000)

	if msg.Type != TypeAudioStreamEnd {
		t.Errorf("Type = %v, want %v", msg.Type, TypeAudioStreamEnd)
	}
	if msg.TotalChunks != 100 {
		t.Errorf("TotalChunks = %v, want %v", msg.TotalChunks, 100)
	}
	if msg.TotalBytes != 512000 {
		t.Errorf("TotalBytes = %v, want %v", msg.TotalBytes, 512000)
	}
	if msg.Cancelled {
		t.Error("Cancelled should be false")
	}
}

func TestNewAudioStreamEndCancelled(t *testing.T) {
	msg := NewAudioStreamEndCancelled("req-123", "stream-456", "user requested")

	if !msg.Cancelled {
		t.Error("Cancelled should be true")
	}
	if msg.CancelReason != "user requested" {
		t.Errorf("CancelReason = %v, want %v", msg.CancelReason, "user requested")
	}
}

func TestAudioStreamEnd_Validate(t *testing.T) {
	tests := []struct {
		name    string
		msg     *AudioStreamEnd
		wantErr bool
	}{
		{
			name:    "valid end",
			msg:     NewAudioStreamEnd("req-123", "stream-456", 100, 512000),
			wantErr: false,
		},
		{
			name:    "valid cancelled",
			msg:     NewAudioStreamEndCancelled("req-123", "stream-456", "reason"),
			wantErr: false,
		},
		{
			name: "cancelled without reason",
			msg: &AudioStreamEnd{
				BaseMessage: BaseMessage{Type: TypeAudioStreamEnd, RequestID: "req-123"},
				StreamID:    "stream-456",
				Cancelled:   true,
			},
			wantErr: true,
		},
		{
			name: "negative total chunks",
			msg: &AudioStreamEnd{
				BaseMessage: BaseMessage{Type: TypeAudioStreamEnd, RequestID: "req-123"},
				StreamID:    "stream-456",
				TotalChunks: -1,
				TotalBytes:  100,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewAudioStreamEndAck(t *testing.T) {
	msg := NewAudioStreamEndAck("req-123", "stream-456", true, 100, 512000)

	if msg.Type != TypeAudioStreamEndAck {
		t.Errorf("Type = %v, want %v", msg.Type, TypeAudioStreamEndAck)
	}
	if !msg.Success {
		t.Error("Success should be true")
	}
	if msg.ChunksReceived != 100 {
		t.Errorf("ChunksReceived = %v, want %v", msg.ChunksReceived, 100)
	}
	if msg.BytesReceived != 512000 {
		t.Errorf("BytesReceived = %v, want %v", msg.BytesReceived, 512000)
	}
}

func TestNewAudioStreamEndAckError(t *testing.T) {
	msg := NewAudioStreamEndAckError("req-123", "stream-456", "processing failed", 50, 256000)

	if msg.Success {
		t.Error("Success should be false")
	}
	if msg.Error != "processing failed" {
		t.Errorf("Error = %v, want %v", msg.Error, "processing failed")
	}
	if msg.ChunksReceived != 50 {
		t.Errorf("ChunksReceived = %v, want %v", msg.ChunksReceived, 50)
	}
}

func TestAudioStreamEndAck_Validate(t *testing.T) {
	tests := []struct {
		name    string
		msg     *AudioStreamEndAck
		wantErr bool
	}{
		{
			name:    "valid success",
			msg:     NewAudioStreamEndAck("req-123", "stream-456", true, 100, 512000),
			wantErr: false,
		},
		{
			name:    "valid error",
			msg:     NewAudioStreamEndAckError("req-123", "stream-456", "error", 50, 256000),
			wantErr: false,
		},
		{
			name: "failure without error",
			msg: &AudioStreamEndAck{
				BaseMessage:    BaseMessage{Type: TypeAudioStreamEndAck, RequestID: "req-123"},
				StreamID:       "stream-456",
				Success:        false,
				ChunksReceived: 50,
				BytesReceived:  256000,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAudioStreamStart_JSON(t *testing.T) {
	msg := NewAudioStreamStartWithOptions("req-123", "stream-456", AudioFormatOpus, 48000, 2, 16, "voice_message")
	msg.ExpectedDurationMs = 30000
	msg.Metadata = map[string]string{"source": "telegram", "user_id": "user-789"}

	// Marshal
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Unmarshal
	var decoded AudioStreamStart
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Verify fields
	if decoded.StreamID != msg.StreamID {
		t.Errorf("StreamID = %v, want %v", decoded.StreamID, msg.StreamID)
	}
	if decoded.Format != msg.Format {
		t.Errorf("Format = %v, want %v", decoded.Format, msg.Format)
	}
	if decoded.SampleRate != msg.SampleRate {
		t.Errorf("SampleRate = %v, want %v", decoded.SampleRate, msg.SampleRate)
	}
	if decoded.ExpectedDurationMs != msg.ExpectedDurationMs {
		t.Errorf("ExpectedDurationMs = %v, want %v", decoded.ExpectedDurationMs, msg.ExpectedDurationMs)
	}
	if decoded.Metadata["source"] != "telegram" {
		t.Errorf("Metadata[source] = %v, want %v", decoded.Metadata["source"], "telegram")
	}
}

func TestAudioStreamChunk_JSON(t *testing.T) {
	data := base64.StdEncoding.EncodeToString([]byte("test audio data"))
	msg := NewAudioStreamChunk("req-123", "stream-456", 5, data)
	msg.DurationMs = 20

	// Marshal
	jsonData, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Unmarshal
	var decoded AudioStreamChunk
	if err := json.Unmarshal(jsonData, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Verify fields
	if decoded.Sequence != 5 {
		t.Errorf("Sequence = %v, want %v", decoded.Sequence, 5)
	}
	if decoded.Data != data {
		t.Errorf("Data = %v, want %v", decoded.Data, data)
	}
	if decoded.DurationMs != 20 {
		t.Errorf("DurationMs = %v, want %v", decoded.DurationMs, 20)
	}
}

func TestAudioStreamEndAck_JSON_WithProcessingResult(t *testing.T) {
	msg := NewAudioStreamEndAck("req-123", "stream-456", true, 100, 512000)
	msg.ProcessingResult = map[string]interface{}{
		"transcription": "Hello world",
		"confidence":    0.95,
		"language":      "en",
	}

	// Marshal
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Unmarshal
	var decoded AudioStreamEndAck
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Verify processing result
	if decoded.ProcessingResult["transcription"] != "Hello world" {
		t.Errorf("ProcessingResult[transcription] = %v, want %v", decoded.ProcessingResult["transcription"], "Hello world")
	}
}

func TestParseMessage_AudioStreamTypes(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		wantType string
	}{
		{
			name:     "audio stream start",
			json:     `{"type":"audio_stream_start","request_id":"req-1","stream_id":"s-1","format":"opus","sample_rate":48000,"channels":2}`,
			wantType: TypeAudioStreamStart,
		},
		{
			name:     "audio stream start ack",
			json:     `{"type":"audio_stream_start_ack","request_id":"req-1","stream_id":"s-1","accepted":true}`,
			wantType: TypeAudioStreamStartAck,
		},
		{
			name:     "audio stream chunk",
			json:     `{"type":"audio_stream_chunk","request_id":"req-1","stream_id":"s-1","sequence":0,"data":"YXVkaW8gZGF0YQ=="}`,
			wantType: TypeAudioStreamChunk,
		},
		{
			name:     "audio stream end",
			json:     `{"type":"audio_stream_end","request_id":"req-1","stream_id":"s-1","total_chunks":10,"total_bytes":1000}`,
			wantType: TypeAudioStreamEnd,
		},
		{
			name:     "audio stream end ack",
			json:     `{"type":"audio_stream_end_ack","request_id":"req-1","stream_id":"s-1","success":true,"chunks_received":10,"bytes_received":1000}`,
			wantType: TypeAudioStreamEndAck,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, msgType, err := ParseMessage([]byte(tt.json))
			if err != nil {
				t.Fatalf("ParseMessage() error = %v", err)
			}
			if msgType != tt.wantType {
				t.Errorf("ParseMessage() type = %v, want %v", msgType, tt.wantType)
			}
			if msg == nil {
				t.Error("ParseMessage() returned nil message")
			}
		})
	}
}

func TestIsValidAudioFormat(t *testing.T) {
	tests := []struct {
		format AudioFormat
		want   bool
	}{
		{AudioFormatPCM16, true},
		{AudioFormatOpus, true},
		{AudioFormatMP3, true},
		{AudioFormatOGG, true},
		{AudioFormatWAV, true},
		{AudioFormatWebM, true},
		{"invalid", false},
		{"", false},
		{"aac", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			if got := IsValidAudioFormat(tt.format); got != tt.want {
				t.Errorf("IsValidAudioFormat(%v) = %v, want %v", tt.format, got, tt.want)
			}
		})
	}
}

func TestGetAudioFormatMimeType(t *testing.T) {
	tests := []struct {
		format   AudioFormat
		wantMime string
	}{
		{AudioFormatPCM16, "audio/pcm"},
		{AudioFormatOpus, "audio/opus"},
		{AudioFormatMP3, "audio/mpeg"},
		{AudioFormatOGG, "audio/ogg"},
		{AudioFormatWAV, "audio/wav"},
		{AudioFormatWebM, "audio/webm"},
		{"unknown", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			if got := GetAudioFormatMimeType(tt.format); got != tt.wantMime {
				t.Errorf("GetAudioFormatMimeType(%v) = %v, want %v", tt.format, got, tt.wantMime)
			}
		})
	}
}

func TestAudioStreamStart_AllFormats(t *testing.T) {
	formats := []AudioFormat{
		AudioFormatPCM16,
		AudioFormatOpus,
		AudioFormatMP3,
		AudioFormatOGG,
		AudioFormatWAV,
		AudioFormatWebM,
	}

	for _, format := range formats {
		t.Run(string(format), func(t *testing.T) {
			msg := NewAudioStreamStart("req-123", "stream-456", format, 44100, 2)
			if err := msg.Validate(); err != nil {
				t.Errorf("Validate() for format %v error = %v", format, err)
			}
		})
	}
}

func TestAudioStreamSequence(t *testing.T) {
	// Simulate a complete audio stream sequence
	streamID := "test-stream-123"
	requestID := "req-"

	// 1. Start stream
	start := NewAudioStreamStart(requestID+"start", streamID, AudioFormatOpus, 48000, 1)
	start.SourceType = "voice_message"
	start.ExpectedDurationMs = 5000
	if err := start.Validate(); err != nil {
		t.Fatalf("Start validation failed: %v", err)
	}

	// 2. Acknowledge start
	startAck := NewAudioStreamStartAck(requestID+"start", streamID, true)
	startAck.MaxChunkSize = 32768
	if err := startAck.Validate(); err != nil {
		t.Fatalf("Start ack validation failed: %v", err)
	}

	// 3. Send chunks
	var totalBytes int64
	for i := int64(0); i < 10; i++ {
		data := base64.StdEncoding.EncodeToString(make([]byte, 1024))
		chunk := NewAudioStreamChunk(requestID+"chunk", streamID, i, data)
		chunk.DurationMs = 100
		if err := chunk.Validate(); err != nil {
			t.Fatalf("Chunk %d validation failed: %v", i, err)
		}
		totalBytes += 1024
	}

	// 4. End stream
	end := NewAudioStreamEnd(requestID+"end", streamID, 10, totalBytes)
	end.TotalDurationMs = 1000
	if err := end.Validate(); err != nil {
		t.Fatalf("End validation failed: %v", err)
	}

	// 5. Acknowledge end
	endAck := NewAudioStreamEndAck(requestID+"end", streamID, true, 10, totalBytes)
	endAck.ProcessingResult = map[string]interface{}{
		"status": "transcribed",
	}
	if err := endAck.Validate(); err != nil {
		t.Fatalf("End ack validation failed: %v", err)
	}
}

func TestAudioStreamCancellation(t *testing.T) {
	streamID := "cancel-stream-123"

	// Start
	start := NewAudioStreamStart("req-1", streamID, AudioFormatMP3, 44100, 2)
	if err := start.Validate(); err != nil {
		t.Fatalf("Start validation failed: %v", err)
	}

	// Acknowledge
	ack := NewAudioStreamStartAck("req-1", streamID, true)
	if err := ack.Validate(); err != nil {
		t.Fatalf("Ack validation failed: %v", err)
	}

	// Cancel
	cancel := NewAudioStreamEndCancelled("req-2", streamID, "user cancelled upload")
	if err := cancel.Validate(); err != nil {
		t.Fatalf("Cancel validation failed: %v", err)
	}

	// Verify cancel fields
	if !cancel.Cancelled {
		t.Error("Expected Cancelled to be true")
	}
	if cancel.CancelReason != "user cancelled upload" {
		t.Errorf("CancelReason = %v, want %v", cancel.CancelReason, "user cancelled upload")
	}
}

func TestAudioStreamRejection(t *testing.T) {
	streamID := "reject-stream-123"

	// Start with unsupported format (simulated by rejection)
	start := NewAudioStreamStart("req-1", streamID, AudioFormatOpus, 48000, 2)
	if err := start.Validate(); err != nil {
		t.Fatalf("Start validation failed: %v", err)
	}

	// Reject
	reject := NewAudioStreamStartAckError("req-1", streamID, "server busy, try again later")
	if err := reject.Validate(); err != nil {
		t.Fatalf("Reject validation failed: %v", err)
	}

	// Verify rejection
	if reject.Accepted {
		t.Error("Expected Accepted to be false")
	}
	if reject.Error != "server busy, try again later" {
		t.Errorf("Error = %v, want %v", reject.Error, "server busy, try again later")
	}
}
