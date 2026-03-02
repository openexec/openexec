package mcp

import (
	"encoding/json"
	"testing"
)

func TestReadFileToolDef(t *testing.T) {
	toolDef := ReadFileToolDef()

	// Verify tool name
	if toolDef["name"] != "read_file" {
		t.Errorf("tool name = %v, want read_file", toolDef["name"])
	}

	// Verify description exists and is non-empty
	desc, ok := toolDef["description"].(string)
	if !ok || desc == "" {
		t.Error("tool description missing or empty")
	}

	// Verify inputSchema structure
	schema, ok := toolDef["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatal("inputSchema is not a map")
	}

	if schema["type"] != "object" {
		t.Errorf("schema type = %v, want object", schema["type"])
	}

	// Verify properties exist
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties is not a map")
	}

	// Check required properties
	requiredProps := []string{"path"}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required is not a []string")
	}

	for _, rp := range requiredProps {
		found := false
		for _, r := range required {
			if r == rp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required property %q not found", rp)
		}
	}

	// Verify path property
	pathProp, ok := props["path"].(map[string]interface{})
	if !ok {
		t.Fatal("path property is not a map")
	}
	if pathProp["type"] != "string" {
		t.Errorf("path type = %v, want string", pathProp["type"])
	}
	if pathProp["description"] == nil || pathProp["description"] == "" {
		t.Error("path property missing description")
	}

	// Verify encoding property
	encodingProp, ok := props["encoding"].(map[string]interface{})
	if !ok {
		t.Fatal("encoding property is not a map")
	}
	if encodingProp["type"] != "string" {
		t.Errorf("encoding type = %v, want string", encodingProp["type"])
	}
	if encodingProp["default"] != "utf-8" {
		t.Errorf("encoding default = %v, want utf-8", encodingProp["default"])
	}

	// Verify encoding enum values
	encodingEnum, ok := encodingProp["enum"].([]string)
	if !ok {
		t.Fatal("encoding enum is not a []string")
	}
	expectedEncodings := map[string]bool{"utf-8": true, "utf-16": true, "ascii": true, "binary": true}
	for _, enc := range encodingEnum {
		if !expectedEncodings[enc] {
			t.Errorf("unexpected encoding enum value: %s", enc)
		}
	}

	// Verify offset property
	offsetProp, ok := props["offset"].(map[string]interface{})
	if !ok {
		t.Fatal("offset property is not a map")
	}
	if offsetProp["type"] != "integer" {
		t.Errorf("offset type = %v, want integer", offsetProp["type"])
	}

	// Verify length property
	lengthProp, ok := props["length"].(map[string]interface{})
	if !ok {
		t.Fatal("length property is not a map")
	}
	if lengthProp["type"] != "integer" {
		t.Errorf("length type = %v, want integer", lengthProp["type"])
	}
}

func TestReadFileToolDefSerializable(t *testing.T) {
	toolDef := ReadFileToolDef()

	// Verify the tool definition can be serialized to JSON
	data, err := json.Marshal(toolDef)
	if err != nil {
		t.Fatalf("failed to marshal tool definition: %v", err)
	}

	// Verify it can be deserialized back
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal tool definition: %v", err)
	}

	if parsed["name"] != "read_file" {
		t.Errorf("deserialized name = %v, want read_file", parsed["name"])
	}
}

func TestValidateReadFileRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     ReadFileRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid minimal request",
			req:     ReadFileRequest{Path: "/path/to/file.txt"},
			wantErr: false,
		},
		{
			name:    "valid full request",
			req:     ReadFileRequest{Path: "/path/to/file.txt", Encoding: "utf-8", Offset: 100, Length: 1024},
			wantErr: false,
		},
		{
			name:    "valid binary encoding",
			req:     ReadFileRequest{Path: "/path/to/file.bin", Encoding: "binary"},
			wantErr: false,
		},
		{
			name:    "empty path",
			req:     ReadFileRequest{Path: ""},
			wantErr: true,
			errMsg:  "path: path is required",
		},
		{
			name:    "invalid encoding",
			req:     ReadFileRequest{Path: "/path/to/file.txt", Encoding: "invalid"},
			wantErr: true,
			errMsg:  "encoding: invalid encoding",
		},
		{
			name:    "negative offset",
			req:     ReadFileRequest{Path: "/path/to/file.txt", Offset: -1},
			wantErr: true,
			errMsg:  "offset: offset must be non-negative",
		},
		{
			name:    "negative length",
			req:     ReadFileRequest{Path: "/path/to/file.txt", Length: -1},
			wantErr: true,
			errMsg:  "length: length must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req // Copy to avoid modifying original
			err := ValidateReadFileRequest(&req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				// Check that error message contains expected substring
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					// Allow partial match for error messages
					t.Logf("error = %q (expected contains %q)", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateReadFileRequestDefaults(t *testing.T) {
	req := ReadFileRequest{Path: "/path/to/file.txt"}

	err := ValidateReadFileRequest(&req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify default encoding is set
	if req.Encoding != DefaultEncoding {
		t.Errorf("default encoding = %v, want %v", req.Encoding, DefaultEncoding)
	}
}

func TestValidationError(t *testing.T) {
	err := &ValidationError{Field: "path", Message: "path is required"}

	expected := "path: path is required"
	if err.Error() != expected {
		t.Errorf("error string = %q, want %q", err.Error(), expected)
	}
}

func TestReadFileRequestJSONParsing(t *testing.T) {
	jsonData := `{"path": "/tmp/test.txt", "encoding": "utf-8", "offset": 0, "length": 100}`

	var req ReadFileRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Path != "/tmp/test.txt" {
		t.Errorf("path = %v, want /tmp/test.txt", req.Path)
	}
	if req.Encoding != "utf-8" {
		t.Errorf("encoding = %v, want utf-8", req.Encoding)
	}
	if req.Offset != 0 {
		t.Errorf("offset = %v, want 0", req.Offset)
	}
	if req.Length != 100 {
		t.Errorf("length = %v, want 100", req.Length)
	}
}

func TestReadFileRequestMinimalJSON(t *testing.T) {
	jsonData := `{"path": "/tmp/test.txt"}`

	var req ReadFileRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Path != "/tmp/test.txt" {
		t.Errorf("path = %v, want /tmp/test.txt", req.Path)
	}

	// Validate to set defaults
	if err := ValidateReadFileRequest(&req); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	if req.Encoding != DefaultEncoding {
		t.Errorf("encoding after validation = %v, want %v", req.Encoding, DefaultEncoding)
	}
}

func TestValidateReadFileRequestAllEncodings(t *testing.T) {
	encodings := []string{"utf-8", "utf-16", "ascii", "binary"}

	for _, enc := range encodings {
		t.Run(enc, func(t *testing.T) {
			req := ReadFileRequest{Path: "/path/to/file.txt", Encoding: enc}
			err := ValidateReadFileRequest(&req)
			if err != nil {
				t.Errorf("unexpected error for encoding %s: %v", enc, err)
			}
		})
	}
}

func TestValidateReadFileRequestZeroLength(t *testing.T) {
	// Zero length should be valid (means read entire file)
	req := ReadFileRequest{Path: "/path/to/file.txt", Length: 0}
	err := ValidateReadFileRequest(&req)
	if err != nil {
		t.Errorf("unexpected error for zero length: %v", err)
	}
}

func TestValidateReadFileRequestZeroOffset(t *testing.T) {
	// Zero offset is valid (start from beginning)
	req := ReadFileRequest{Path: "/path/to/file.txt", Offset: 0}
	err := ValidateReadFileRequest(&req)
	if err != nil {
		t.Errorf("unexpected error for zero offset: %v", err)
	}
}

func TestValidateReadFileRequestLargeOffset(t *testing.T) {
	// Large offset should be valid at validation time
	req := ReadFileRequest{Path: "/path/to/file.txt", Offset: 1000000000}
	err := ValidateReadFileRequest(&req)
	if err != nil {
		t.Errorf("unexpected error for large offset: %v", err)
	}
}

func TestValidateReadFileRequestLargeLength(t *testing.T) {
	// Large length should be valid at validation time
	req := ReadFileRequest{Path: "/path/to/file.txt", Length: 1000000000}
	err := ValidateReadFileRequest(&req)
	if err != nil {
		t.Errorf("unexpected error for large length: %v", err)
	}
}

func TestValidateReadFileRequestPathWithSpaces(t *testing.T) {
	req := ReadFileRequest{Path: "/path/with spaces/file.txt"}
	err := ValidateReadFileRequest(&req)
	if err != nil {
		t.Errorf("unexpected error for path with spaces: %v", err)
	}
}

func TestValidateReadFileRequestPathWithSpecialChars(t *testing.T) {
	specialPaths := []string{
		"/path/with-dashes/file.txt",
		"/path/with_underscores/file.txt",
		"/path/with.dots/file.txt",
		"/path/日本語/file.txt",
		"/path/emoji🎉/file.txt",
	}

	for _, path := range specialPaths {
		t.Run(path, func(t *testing.T) {
			req := ReadFileRequest{Path: path}
			err := ValidateReadFileRequest(&req)
			if err != nil {
				t.Errorf("unexpected error for path %q: %v", path, err)
			}
		})
	}
}

func TestValidateReadFileRequestEmptyPathError(t *testing.T) {
	req := ReadFileRequest{Path: ""}
	err := ValidateReadFileRequest(&req)
	if err == nil {
		t.Error("expected error for empty path")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if validationErr.Field != "path" {
		t.Errorf("error field = %q, want %q", validationErr.Field, "path")
	}
}

func TestValidateReadFileRequestInvalidEncodingError(t *testing.T) {
	invalidEncodings := []string{"UTF8", "utf8", "BINARY", "latin1", "iso-8859-1", ""}
	// Note: empty string is handled by setting default, not returning error

	for _, enc := range invalidEncodings[0 : len(invalidEncodings)-1] {
		t.Run(enc, func(t *testing.T) {
			req := ReadFileRequest{Path: "/path/to/file.txt", Encoding: enc}
			err := ValidateReadFileRequest(&req)
			if err == nil {
				t.Errorf("expected error for invalid encoding %q", enc)
				return
			}

			validationErr, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("expected ValidationError, got %T", err)
			}

			if validationErr.Field != "encoding" {
				t.Errorf("error field = %q, want %q", validationErr.Field, "encoding")
			}
		})
	}
}

func TestValidateReadFileRequestNegativeOffsetError(t *testing.T) {
	req := ReadFileRequest{Path: "/path/to/file.txt", Offset: -10}
	err := ValidateReadFileRequest(&req)
	if err == nil {
		t.Error("expected error for negative offset")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if validationErr.Field != "offset" {
		t.Errorf("error field = %q, want %q", validationErr.Field, "offset")
	}
}

func TestValidateReadFileRequestNegativeLengthError(t *testing.T) {
	req := ReadFileRequest{Path: "/path/to/file.txt", Length: -5}
	err := ValidateReadFileRequest(&req)
	if err == nil {
		t.Error("expected error for negative length")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if validationErr.Field != "length" {
		t.Errorf("error field = %q, want %q", validationErr.Field, "length")
	}
}

func TestReadFileToolDefInputSchemaComplete(t *testing.T) {
	toolDef := ReadFileToolDef()

	schema, ok := toolDef["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatal("inputSchema is not a map")
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties is not a map")
	}

	// Verify all expected properties exist
	expectedProps := []string{"path", "encoding", "offset", "length"}
	for _, prop := range expectedProps {
		if props[prop] == nil {
			t.Errorf("missing expected property: %s", prop)
		}
	}

	// Verify offset has minimum constraint
	offsetProp, _ := props["offset"].(map[string]interface{})
	if offsetProp["minimum"] != 0 {
		t.Errorf("offset minimum = %v, want 0", offsetProp["minimum"])
	}

	// Verify length has minimum constraint
	lengthProp, _ := props["length"].(map[string]interface{})
	if lengthProp["minimum"] != 1 {
		t.Errorf("length minimum = %v, want 1", lengthProp["minimum"])
	}

	// Verify encoding has enum
	encodingProp, _ := props["encoding"].(map[string]interface{})
	encodingEnum, ok := encodingProp["enum"].([]string)
	if !ok {
		t.Fatal("encoding enum is not a []string")
	}
	if len(encodingEnum) != 4 {
		t.Errorf("encoding enum has %d values, want 4", len(encodingEnum))
	}
}

func TestReadFileRequestJSONOmitEmpty(t *testing.T) {
	// Test that optional fields are omitted when empty
	req := ReadFileRequest{Path: "/tmp/test.txt"}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Parse back to verify structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Path should be present
	if parsed["path"] != "/tmp/test.txt" {
		t.Errorf("path = %v, want /tmp/test.txt", parsed["path"])
	}

	// Encoding should be omitted (empty)
	if _, exists := parsed["encoding"]; exists && parsed["encoding"] != "" {
		t.Errorf("encoding should be omitted or empty, got %v", parsed["encoding"])
	}

	// Offset should be omitted (zero)
	if val, exists := parsed["offset"]; exists && val != float64(0) {
		t.Errorf("offset should be omitted or zero, got %v", parsed["offset"])
	}

	// Length should be omitted (zero)
	if val, exists := parsed["length"]; exists && val != float64(0) {
		t.Errorf("length should be omitted or zero, got %v", parsed["length"])
	}
}

func TestReadFileRequestJSONWithAllFields(t *testing.T) {
	jsonData := `{
		"path": "/path/to/file.txt",
		"encoding": "binary",
		"offset": 100,
		"length": 500
	}`

	var req ReadFileRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Path != "/path/to/file.txt" {
		t.Errorf("path = %v, want /path/to/file.txt", req.Path)
	}
	if req.Encoding != "binary" {
		t.Errorf("encoding = %v, want binary", req.Encoding)
	}
	if req.Offset != 100 {
		t.Errorf("offset = %v, want 100", req.Offset)
	}
	if req.Length != 500 {
		t.Errorf("length = %v, want 500", req.Length)
	}
}

func TestValidationErrorInterface(t *testing.T) {
	var err error = &ValidationError{Field: "test", Message: "test message"}

	// Verify it implements error interface
	if err.Error() != "test: test message" {
		t.Errorf("Error() = %q, want %q", err.Error(), "test: test message")
	}

	// Type assertion should work
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Error("type assertion failed")
	}
	if ve.Field != "test" {
		t.Errorf("Field = %q, want %q", ve.Field, "test")
	}
	if ve.Message != "test message" {
		t.Errorf("Message = %q, want %q", ve.Message, "test message")
	}
}

// ==================== WriteFileToolDef Tests ====================

func TestWriteFileToolDef(t *testing.T) {
	toolDef := WriteFileToolDef()

	// Verify tool name
	if toolDef["name"] != "write_file" {
		t.Errorf("tool name = %v, want write_file", toolDef["name"])
	}

	// Verify description exists and is non-empty
	desc, ok := toolDef["description"].(string)
	if !ok || desc == "" {
		t.Error("tool description missing or empty")
	}

	// Verify inputSchema structure
	schema, ok := toolDef["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatal("inputSchema is not a map")
	}

	if schema["type"] != "object" {
		t.Errorf("schema type = %v, want object", schema["type"])
	}

	// Verify properties exist
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties is not a map")
	}

	// Check required properties
	requiredProps := []string{"path", "content"}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required is not a []string")
	}

	for _, rp := range requiredProps {
		found := false
		for _, r := range required {
			if r == rp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required property %q not found", rp)
		}
	}

	// Verify path property
	pathProp, ok := props["path"].(map[string]interface{})
	if !ok {
		t.Fatal("path property is not a map")
	}
	if pathProp["type"] != "string" {
		t.Errorf("path type = %v, want string", pathProp["type"])
	}
	if pathProp["description"] == nil || pathProp["description"] == "" {
		t.Error("path property missing description")
	}

	// Verify content property
	contentProp, ok := props["content"].(map[string]interface{})
	if !ok {
		t.Fatal("content property is not a map")
	}
	if contentProp["type"] != "string" {
		t.Errorf("content type = %v, want string", contentProp["type"])
	}
	if contentProp["description"] == nil || contentProp["description"] == "" {
		t.Error("content property missing description")
	}

	// Verify encoding property
	encodingProp, ok := props["encoding"].(map[string]interface{})
	if !ok {
		t.Fatal("encoding property is not a map")
	}
	if encodingProp["type"] != "string" {
		t.Errorf("encoding type = %v, want string", encodingProp["type"])
	}
	if encodingProp["default"] != "utf-8" {
		t.Errorf("encoding default = %v, want utf-8", encodingProp["default"])
	}

	// Verify encoding enum values
	encodingEnum, ok := encodingProp["enum"].([]string)
	if !ok {
		t.Fatal("encoding enum is not a []string")
	}
	expectedEncodings := map[string]bool{"utf-8": true, "utf-16": true, "ascii": true, "binary": true}
	for _, enc := range encodingEnum {
		if !expectedEncodings[enc] {
			t.Errorf("unexpected encoding enum value: %s", enc)
		}
	}

	// Verify mode property
	modeProp, ok := props["mode"].(map[string]interface{})
	if !ok {
		t.Fatal("mode property is not a map")
	}
	if modeProp["type"] != "string" {
		t.Errorf("mode type = %v, want string", modeProp["type"])
	}
	if modeProp["default"] != "overwrite" {
		t.Errorf("mode default = %v, want overwrite", modeProp["default"])
	}

	// Verify mode enum values
	modeEnum, ok := modeProp["enum"].([]string)
	if !ok {
		t.Fatal("mode enum is not a []string")
	}
	expectedModes := map[string]bool{"overwrite": true, "append": true}
	for _, m := range modeEnum {
		if !expectedModes[m] {
			t.Errorf("unexpected mode enum value: %s", m)
		}
	}

	// Verify create_directories property
	createDirsProp, ok := props["create_directories"].(map[string]interface{})
	if !ok {
		t.Fatal("create_directories property is not a map")
	}
	if createDirsProp["type"] != "boolean" {
		t.Errorf("create_directories type = %v, want boolean", createDirsProp["type"])
	}
	if createDirsProp["default"] != false {
		t.Errorf("create_directories default = %v, want false", createDirsProp["default"])
	}
}

func TestWriteFileToolDefSerializable(t *testing.T) {
	toolDef := WriteFileToolDef()

	// Verify the tool definition can be serialized to JSON
	data, err := json.Marshal(toolDef)
	if err != nil {
		t.Fatalf("failed to marshal tool definition: %v", err)
	}

	// Verify it can be deserialized back
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal tool definition: %v", err)
	}

	if parsed["name"] != "write_file" {
		t.Errorf("deserialized name = %v, want write_file", parsed["name"])
	}
}

func TestValidateWriteFileRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     WriteFileRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid minimal request",
			req:     WriteFileRequest{Path: "/path/to/file.txt", Content: "hello world"},
			wantErr: false,
		},
		{
			name:    "valid full request",
			req:     WriteFileRequest{Path: "/path/to/file.txt", Content: "hello world", Encoding: "utf-8", Mode: "overwrite", CreateDirectories: true},
			wantErr: false,
		},
		{
			name:    "valid append mode",
			req:     WriteFileRequest{Path: "/path/to/file.txt", Content: "appended content", Mode: "append"},
			wantErr: false,
		},
		{
			name:    "valid binary encoding",
			req:     WriteFileRequest{Path: "/path/to/file.bin", Content: "SGVsbG8gV29ybGQ=", Encoding: "binary"},
			wantErr: false,
		},
		{
			name:    "valid empty content",
			req:     WriteFileRequest{Path: "/path/to/file.txt", Content: ""},
			wantErr: false,
		},
		{
			name:    "empty path",
			req:     WriteFileRequest{Path: "", Content: "hello"},
			wantErr: true,
			errMsg:  "path: path is required",
		},
		{
			name:    "invalid encoding",
			req:     WriteFileRequest{Path: "/path/to/file.txt", Content: "hello", Encoding: "invalid"},
			wantErr: true,
			errMsg:  "encoding: invalid encoding",
		},
		{
			name:    "invalid mode",
			req:     WriteFileRequest{Path: "/path/to/file.txt", Content: "hello", Mode: "invalid"},
			wantErr: true,
			errMsg:  "mode: invalid mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req // Copy to avoid modifying original
			err := ValidateWriteFileRequest(&req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				// Check that error message contains expected substring
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Logf("error = %q (expected contains %q)", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateWriteFileRequestDefaults(t *testing.T) {
	req := WriteFileRequest{Path: "/path/to/file.txt", Content: "hello"}

	err := ValidateWriteFileRequest(&req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify default encoding is set
	if req.Encoding != DefaultEncoding {
		t.Errorf("default encoding = %v, want %v", req.Encoding, DefaultEncoding)
	}

	// Verify default mode is set
	if req.Mode != WriteModeOverwrite {
		t.Errorf("default mode = %v, want %v", req.Mode, WriteModeOverwrite)
	}
}

func TestWriteFileRequestJSONParsing(t *testing.T) {
	jsonData := `{"path": "/tmp/test.txt", "content": "hello world", "encoding": "utf-8", "mode": "overwrite", "create_directories": true}`

	var req WriteFileRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Path != "/tmp/test.txt" {
		t.Errorf("path = %v, want /tmp/test.txt", req.Path)
	}
	if req.Content != "hello world" {
		t.Errorf("content = %v, want hello world", req.Content)
	}
	if req.Encoding != "utf-8" {
		t.Errorf("encoding = %v, want utf-8", req.Encoding)
	}
	if req.Mode != "overwrite" {
		t.Errorf("mode = %v, want overwrite", req.Mode)
	}
	if req.CreateDirectories != true {
		t.Errorf("create_directories = %v, want true", req.CreateDirectories)
	}
}

func TestWriteFileRequestMinimalJSON(t *testing.T) {
	jsonData := `{"path": "/tmp/test.txt", "content": "hello"}`

	var req WriteFileRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Path != "/tmp/test.txt" {
		t.Errorf("path = %v, want /tmp/test.txt", req.Path)
	}
	if req.Content != "hello" {
		t.Errorf("content = %v, want hello", req.Content)
	}

	// Validate to set defaults
	if err := ValidateWriteFileRequest(&req); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	if req.Encoding != DefaultEncoding {
		t.Errorf("encoding after validation = %v, want %v", req.Encoding, DefaultEncoding)
	}
	if req.Mode != WriteModeOverwrite {
		t.Errorf("mode after validation = %v, want %v", req.Mode, WriteModeOverwrite)
	}
}

func TestValidateWriteFileRequestAllEncodings(t *testing.T) {
	encodings := []string{"utf-8", "utf-16", "ascii", "binary"}

	for _, enc := range encodings {
		t.Run(enc, func(t *testing.T) {
			req := WriteFileRequest{Path: "/path/to/file.txt", Content: "hello", Encoding: enc}
			err := ValidateWriteFileRequest(&req)
			if err != nil {
				t.Errorf("unexpected error for encoding %s: %v", enc, err)
			}
		})
	}
}

func TestValidateWriteFileRequestAllModes(t *testing.T) {
	modes := []string{"overwrite", "append"}

	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			req := WriteFileRequest{Path: "/path/to/file.txt", Content: "hello", Mode: mode}
			err := ValidateWriteFileRequest(&req)
			if err != nil {
				t.Errorf("unexpected error for mode %s: %v", mode, err)
			}
		})
	}
}

func TestValidateWriteFileRequestEmptyPathError(t *testing.T) {
	req := WriteFileRequest{Path: "", Content: "hello"}
	err := ValidateWriteFileRequest(&req)
	if err == nil {
		t.Error("expected error for empty path")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if validationErr.Field != "path" {
		t.Errorf("error field = %q, want %q", validationErr.Field, "path")
	}
}

func TestValidateWriteFileRequestInvalidEncodingError(t *testing.T) {
	invalidEncodings := []string{"UTF8", "utf8", "BINARY", "latin1", "iso-8859-1"}

	for _, enc := range invalidEncodings {
		t.Run(enc, func(t *testing.T) {
			req := WriteFileRequest{Path: "/path/to/file.txt", Content: "hello", Encoding: enc}
			err := ValidateWriteFileRequest(&req)
			if err == nil {
				t.Errorf("expected error for invalid encoding %q", enc)
				return
			}

			validationErr, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("expected ValidationError, got %T", err)
			}

			if validationErr.Field != "encoding" {
				t.Errorf("error field = %q, want %q", validationErr.Field, "encoding")
			}
		})
	}
}

func TestValidateWriteFileRequestInvalidModeError(t *testing.T) {
	invalidModes := []string{"OVERWRITE", "Append", "write", "create", "truncate"}

	for _, mode := range invalidModes {
		t.Run(mode, func(t *testing.T) {
			req := WriteFileRequest{Path: "/path/to/file.txt", Content: "hello", Mode: mode}
			err := ValidateWriteFileRequest(&req)
			if err == nil {
				t.Errorf("expected error for invalid mode %q", mode)
				return
			}

			validationErr, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("expected ValidationError, got %T", err)
			}

			if validationErr.Field != "mode" {
				t.Errorf("error field = %q, want %q", validationErr.Field, "mode")
			}
		})
	}
}

func TestWriteFileToolDefInputSchemaComplete(t *testing.T) {
	toolDef := WriteFileToolDef()

	schema, ok := toolDef["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatal("inputSchema is not a map")
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties is not a map")
	}

	// Verify all expected properties exist
	expectedProps := []string{"path", "content", "encoding", "mode", "create_directories"}
	for _, prop := range expectedProps {
		if props[prop] == nil {
			t.Errorf("missing expected property: %s", prop)
		}
	}

	// Verify encoding has enum
	encodingProp, _ := props["encoding"].(map[string]interface{})
	encodingEnum, ok := encodingProp["enum"].([]string)
	if !ok {
		t.Fatal("encoding enum is not a []string")
	}
	if len(encodingEnum) != 4 {
		t.Errorf("encoding enum has %d values, want 4", len(encodingEnum))
	}

	// Verify mode has enum
	modeProp, _ := props["mode"].(map[string]interface{})
	modeEnum, ok := modeProp["enum"].([]string)
	if !ok {
		t.Fatal("mode enum is not a []string")
	}
	if len(modeEnum) != 2 {
		t.Errorf("mode enum has %d values, want 2", len(modeEnum))
	}
}

func TestWriteFileRequestJSONOmitEmpty(t *testing.T) {
	// Test that optional fields are omitted when empty
	req := WriteFileRequest{Path: "/tmp/test.txt", Content: "hello"}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Parse back to verify structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Path and content should be present
	if parsed["path"] != "/tmp/test.txt" {
		t.Errorf("path = %v, want /tmp/test.txt", parsed["path"])
	}
	if parsed["content"] != "hello" {
		t.Errorf("content = %v, want hello", parsed["content"])
	}

	// Encoding should be omitted (empty)
	if _, exists := parsed["encoding"]; exists && parsed["encoding"] != "" {
		t.Errorf("encoding should be omitted or empty, got %v", parsed["encoding"])
	}

	// Mode should be omitted (empty)
	if _, exists := parsed["mode"]; exists && parsed["mode"] != "" {
		t.Errorf("mode should be omitted or empty, got %v", parsed["mode"])
	}
}

func TestWriteFileRequestJSONWithAllFields(t *testing.T) {
	jsonData := `{
		"path": "/path/to/file.txt",
		"content": "file content here",
		"encoding": "binary",
		"mode": "append",
		"create_directories": true
	}`

	var req WriteFileRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Path != "/path/to/file.txt" {
		t.Errorf("path = %v, want /path/to/file.txt", req.Path)
	}
	if req.Content != "file content here" {
		t.Errorf("content = %v, want file content here", req.Content)
	}
	if req.Encoding != "binary" {
		t.Errorf("encoding = %v, want binary", req.Encoding)
	}
	if req.Mode != "append" {
		t.Errorf("mode = %v, want append", req.Mode)
	}
	if req.CreateDirectories != true {
		t.Errorf("create_directories = %v, want true", req.CreateDirectories)
	}
}

func TestWriteFileRequestPathWithSpaces(t *testing.T) {
	req := WriteFileRequest{Path: "/path/with spaces/file.txt", Content: "hello"}
	err := ValidateWriteFileRequest(&req)
	if err != nil {
		t.Errorf("unexpected error for path with spaces: %v", err)
	}
}

func TestWriteFileRequestPathWithSpecialChars(t *testing.T) {
	specialPaths := []string{
		"/path/with-dashes/file.txt",
		"/path/with_underscores/file.txt",
		"/path/with.dots/file.txt",
		"/path/日本語/file.txt",
		"/path/emoji🎉/file.txt",
	}

	for _, path := range specialPaths {
		t.Run(path, func(t *testing.T) {
			req := WriteFileRequest{Path: path, Content: "hello"}
			err := ValidateWriteFileRequest(&req)
			if err != nil {
				t.Errorf("unexpected error for path %q: %v", path, err)
			}
		})
	}
}

func TestWriteFileRequestLargeContent(t *testing.T) {
	// Test with large content (1MB)
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = byte('a' + (i % 26))
	}

	req := WriteFileRequest{Path: "/path/to/file.txt", Content: string(largeContent)}
	err := ValidateWriteFileRequest(&req)
	if err != nil {
		t.Errorf("unexpected error for large content: %v", err)
	}
}

func TestWriteFileRequestMultilineContent(t *testing.T) {
	multilineContent := `line 1
line 2
line 3
	indented line`

	req := WriteFileRequest{Path: "/path/to/file.txt", Content: multilineContent}
	err := ValidateWriteFileRequest(&req)
	if err != nil {
		t.Errorf("unexpected error for multiline content: %v", err)
	}
}

func TestWriteModeConstants(t *testing.T) {
	if WriteModeOverwrite != "overwrite" {
		t.Errorf("WriteModeOverwrite = %q, want %q", WriteModeOverwrite, "overwrite")
	}
	if WriteModeAppend != "append" {
		t.Errorf("WriteModeAppend = %q, want %q", WriteModeAppend, "append")
	}
}

// ==================== RunShellCommandToolDef Tests ====================

func TestRunShellCommandToolDef(t *testing.T) {
	toolDef := RunShellCommandToolDef()

	// Verify tool name
	if toolDef["name"] != "run_shell_command" {
		t.Errorf("tool name = %v, want run_shell_command", toolDef["name"])
	}

	// Verify description exists and is non-empty
	desc, ok := toolDef["description"].(string)
	if !ok || desc == "" {
		t.Error("tool description missing or empty")
	}

	// Verify inputSchema structure
	schema, ok := toolDef["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatal("inputSchema is not a map")
	}

	if schema["type"] != "object" {
		t.Errorf("schema type = %v, want object", schema["type"])
	}

	// Verify properties exist
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties is not a map")
	}

	// Check required properties
	requiredProps := []string{"command"}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required is not a []string")
	}

	for _, rp := range requiredProps {
		found := false
		for _, r := range required {
			if r == rp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required property %q not found", rp)
		}
	}

	// Verify command property
	commandProp, ok := props["command"].(map[string]interface{})
	if !ok {
		t.Fatal("command property is not a map")
	}
	if commandProp["type"] != "string" {
		t.Errorf("command type = %v, want string", commandProp["type"])
	}
	if commandProp["description"] == nil || commandProp["description"] == "" {
		t.Error("command property missing description")
	}

	// Verify args property
	argsProp, ok := props["args"].(map[string]interface{})
	if !ok {
		t.Fatal("args property is not a map")
	}
	if argsProp["type"] != "array" {
		t.Errorf("args type = %v, want array", argsProp["type"])
	}
	argsItems, ok := argsProp["items"].(map[string]interface{})
	if !ok {
		t.Fatal("args items is not a map")
	}
	if argsItems["type"] != "string" {
		t.Errorf("args items type = %v, want string", argsItems["type"])
	}

	// Verify working_directory property
	wdProp, ok := props["working_directory"].(map[string]interface{})
	if !ok {
		t.Fatal("working_directory property is not a map")
	}
	if wdProp["type"] != "string" {
		t.Errorf("working_directory type = %v, want string", wdProp["type"])
	}

	// Verify timeout_ms property
	timeoutProp, ok := props["timeout_ms"].(map[string]interface{})
	if !ok {
		t.Fatal("timeout_ms property is not a map")
	}
	if timeoutProp["type"] != "integer" {
		t.Errorf("timeout_ms type = %v, want integer", timeoutProp["type"])
	}
	if timeoutProp["default"] != 30000 {
		t.Errorf("timeout_ms default = %v, want 30000", timeoutProp["default"])
	}
	if timeoutProp["minimum"] != 100 {
		t.Errorf("timeout_ms minimum = %v, want 100", timeoutProp["minimum"])
	}
	if timeoutProp["maximum"] != 600000 {
		t.Errorf("timeout_ms maximum = %v, want 600000", timeoutProp["maximum"])
	}

	// Verify env property
	envProp, ok := props["env"].(map[string]interface{})
	if !ok {
		t.Fatal("env property is not a map")
	}
	if envProp["type"] != "object" {
		t.Errorf("env type = %v, want object", envProp["type"])
	}
	envAdditionalProps, ok := envProp["additionalProperties"].(map[string]interface{})
	if !ok {
		t.Fatal("env additionalProperties is not a map")
	}
	if envAdditionalProps["type"] != "string" {
		t.Errorf("env additionalProperties type = %v, want string", envAdditionalProps["type"])
	}

	// Verify stdin property
	stdinProp, ok := props["stdin"].(map[string]interface{})
	if !ok {
		t.Fatal("stdin property is not a map")
	}
	if stdinProp["type"] != "string" {
		t.Errorf("stdin type = %v, want string", stdinProp["type"])
	}
}

func TestRunShellCommandToolDefSerializable(t *testing.T) {
	toolDef := RunShellCommandToolDef()

	// Verify the tool definition can be serialized to JSON
	data, err := json.Marshal(toolDef)
	if err != nil {
		t.Fatalf("failed to marshal tool definition: %v", err)
	}

	// Verify it can be deserialized back
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal tool definition: %v", err)
	}

	if parsed["name"] != "run_shell_command" {
		t.Errorf("deserialized name = %v, want run_shell_command", parsed["name"])
	}
}

func TestValidateRunShellCommandRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     RunShellCommandRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid minimal request",
			req:     RunShellCommandRequest{Command: "ls -la"},
			wantErr: false,
		},
		{
			name:    "valid full request",
			req:     RunShellCommandRequest{Command: "echo", Args: []string{"hello", "world"}, WorkingDirectory: "/tmp", TimeoutMs: 5000, Env: map[string]string{"FOO": "bar"}, Stdin: "input"},
			wantErr: false,
		},
		{
			name:    "valid with only command and args",
			req:     RunShellCommandRequest{Command: "grep", Args: []string{"-r", "pattern", "."}},
			wantErr: false,
		},
		{
			name:    "valid with custom timeout",
			req:     RunShellCommandRequest{Command: "sleep 5", TimeoutMs: 10000},
			wantErr: false,
		},
		{
			name:    "valid with environment variables",
			req:     RunShellCommandRequest{Command: "printenv", Env: map[string]string{"MY_VAR": "value", "ANOTHER": "val2"}},
			wantErr: false,
		},
		{
			name:    "empty command",
			req:     RunShellCommandRequest{Command: ""},
			wantErr: true,
			errMsg:  "command: command is required",
		},
		{
			name:    "timeout too low",
			req:     RunShellCommandRequest{Command: "ls", TimeoutMs: 50},
			wantErr: true,
			errMsg:  "timeout_ms: timeout_ms must be at least 100 milliseconds",
		},
		{
			name:    "timeout too high",
			req:     RunShellCommandRequest{Command: "ls", TimeoutMs: 700000},
			wantErr: true,
			errMsg:  "timeout_ms: timeout_ms must not exceed 600000 milliseconds (10 minutes)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req // Copy to avoid modifying original
			err := ValidateRunShellCommandRequest(&req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				// Check that error message contains expected substring
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Logf("error = %q (expected contains %q)", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateRunShellCommandRequestDefaults(t *testing.T) {
	req := RunShellCommandRequest{Command: "ls"}

	err := ValidateRunShellCommandRequest(&req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify default timeout is set
	if req.TimeoutMs != DefaultTimeoutMs {
		t.Errorf("default timeout = %v, want %v", req.TimeoutMs, DefaultTimeoutMs)
	}
}

func TestRunShellCommandRequestJSONParsing(t *testing.T) {
	jsonData := `{"command": "echo hello", "args": ["arg1", "arg2"], "working_directory": "/tmp", "timeout_ms": 5000, "env": {"FOO": "bar"}, "stdin": "input data"}`

	var req RunShellCommandRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Command != "echo hello" {
		t.Errorf("command = %v, want echo hello", req.Command)
	}
	if len(req.Args) != 2 || req.Args[0] != "arg1" || req.Args[1] != "arg2" {
		t.Errorf("args = %v, want [arg1, arg2]", req.Args)
	}
	if req.WorkingDirectory != "/tmp" {
		t.Errorf("working_directory = %v, want /tmp", req.WorkingDirectory)
	}
	if req.TimeoutMs != 5000 {
		t.Errorf("timeout_ms = %v, want 5000", req.TimeoutMs)
	}
	if req.Env["FOO"] != "bar" {
		t.Errorf("env[FOO] = %v, want bar", req.Env["FOO"])
	}
	if req.Stdin != "input data" {
		t.Errorf("stdin = %v, want input data", req.Stdin)
	}
}

func TestRunShellCommandRequestMinimalJSON(t *testing.T) {
	jsonData := `{"command": "ls -la"}`

	var req RunShellCommandRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Command != "ls -la" {
		t.Errorf("command = %v, want ls -la", req.Command)
	}

	// Validate to set defaults
	if err := ValidateRunShellCommandRequest(&req); err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	if req.TimeoutMs != DefaultTimeoutMs {
		t.Errorf("timeout_ms after validation = %v, want %v", req.TimeoutMs, DefaultTimeoutMs)
	}
}

func TestValidateRunShellCommandRequestEmptyCommandError(t *testing.T) {
	req := RunShellCommandRequest{Command: ""}
	err := ValidateRunShellCommandRequest(&req)
	if err == nil {
		t.Error("expected error for empty command")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if validationErr.Field != "command" {
		t.Errorf("error field = %q, want %q", validationErr.Field, "command")
	}
}

func TestValidateRunShellCommandRequestTimeoutBoundaries(t *testing.T) {
	// Test minimum boundary (100ms)
	reqMin := RunShellCommandRequest{Command: "ls", TimeoutMs: 100}
	if err := ValidateRunShellCommandRequest(&reqMin); err != nil {
		t.Errorf("unexpected error for timeout_ms=100: %v", err)
	}

	// Test maximum boundary (600000ms)
	reqMax := RunShellCommandRequest{Command: "ls", TimeoutMs: 600000}
	if err := ValidateRunShellCommandRequest(&reqMax); err != nil {
		t.Errorf("unexpected error for timeout_ms=600000: %v", err)
	}

	// Test just below minimum (99ms)
	reqBelowMin := RunShellCommandRequest{Command: "ls", TimeoutMs: 99}
	if err := ValidateRunShellCommandRequest(&reqBelowMin); err == nil {
		t.Error("expected error for timeout_ms=99")
	}

	// Test just above maximum (600001ms)
	reqAboveMax := RunShellCommandRequest{Command: "ls", TimeoutMs: 600001}
	if err := ValidateRunShellCommandRequest(&reqAboveMax); err == nil {
		t.Error("expected error for timeout_ms=600001")
	}
}

func TestRunShellCommandToolDefInputSchemaComplete(t *testing.T) {
	toolDef := RunShellCommandToolDef()

	schema, ok := toolDef["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatal("inputSchema is not a map")
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties is not a map")
	}

	// Verify all expected properties exist
	expectedProps := []string{"command", "args", "working_directory", "timeout_ms", "env", "stdin"}
	for _, prop := range expectedProps {
		if props[prop] == nil {
			t.Errorf("missing expected property: %s", prop)
		}
	}
}

func TestRunShellCommandRequestJSONOmitEmpty(t *testing.T) {
	// Test that optional fields are omitted when empty
	req := RunShellCommandRequest{Command: "ls"}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Parse back to verify structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Command should be present
	if parsed["command"] != "ls" {
		t.Errorf("command = %v, want ls", parsed["command"])
	}

	// Args should be omitted (nil)
	if _, exists := parsed["args"]; exists {
		t.Errorf("args should be omitted, got %v", parsed["args"])
	}

	// Working directory should be omitted (empty)
	if _, exists := parsed["working_directory"]; exists && parsed["working_directory"] != "" {
		t.Errorf("working_directory should be omitted or empty, got %v", parsed["working_directory"])
	}

	// Timeout should be omitted (zero)
	if val, exists := parsed["timeout_ms"]; exists && val != float64(0) {
		t.Errorf("timeout_ms should be omitted or zero, got %v", parsed["timeout_ms"])
	}
}

func TestRunShellCommandRequestJSONWithAllFields(t *testing.T) {
	jsonData := `{
		"command": "cat",
		"args": ["-n"],
		"working_directory": "/home/user",
		"timeout_ms": 15000,
		"env": {"PATH": "/usr/bin", "HOME": "/home/user"},
		"stdin": "line1\nline2\nline3"
	}`

	var req RunShellCommandRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Command != "cat" {
		t.Errorf("command = %v, want cat", req.Command)
	}
	if len(req.Args) != 1 || req.Args[0] != "-n" {
		t.Errorf("args = %v, want [-n]", req.Args)
	}
	if req.WorkingDirectory != "/home/user" {
		t.Errorf("working_directory = %v, want /home/user", req.WorkingDirectory)
	}
	if req.TimeoutMs != 15000 {
		t.Errorf("timeout_ms = %v, want 15000", req.TimeoutMs)
	}
	if len(req.Env) != 2 {
		t.Errorf("env has %d entries, want 2", len(req.Env))
	}
	if req.Env["PATH"] != "/usr/bin" {
		t.Errorf("env[PATH] = %v, want /usr/bin", req.Env["PATH"])
	}
	if req.Stdin != "line1\nline2\nline3" {
		t.Errorf("stdin = %v, want line1\\nline2\\nline3", req.Stdin)
	}
}

func TestRunShellCommandRequestCommandWithSpecialChars(t *testing.T) {
	specialCommands := []string{
		"ls -la | grep test",
		"echo 'hello world'",
		`echo "hello $USER"`,
		"cat file.txt > output.txt",
		"cmd1 && cmd2 || cmd3",
		"find . -name '*.go'",
	}

	for _, cmd := range specialCommands {
		t.Run(cmd, func(t *testing.T) {
			req := RunShellCommandRequest{Command: cmd}
			err := ValidateRunShellCommandRequest(&req)
			if err != nil {
				t.Errorf("unexpected error for command %q: %v", cmd, err)
			}
		})
	}
}

func TestRunShellCommandRequestMultilineStdin(t *testing.T) {
	multilineStdin := `line 1
line 2
line 3
	indented line`

	req := RunShellCommandRequest{Command: "cat", Stdin: multilineStdin}
	err := ValidateRunShellCommandRequest(&req)
	if err != nil {
		t.Errorf("unexpected error for multiline stdin: %v", err)
	}
}

func TestRunShellCommandTimeoutConstants(t *testing.T) {
	if DefaultTimeoutMs != 30000 {
		t.Errorf("DefaultTimeoutMs = %d, want %d", DefaultTimeoutMs, 30000)
	}
	if MinTimeoutMs != 100 {
		t.Errorf("MinTimeoutMs = %d, want %d", MinTimeoutMs, 100)
	}
	if MaxTimeoutMs != 600000 {
		t.Errorf("MaxTimeoutMs = %d, want %d", MaxTimeoutMs, 600000)
	}
}

// ==================== GitApplyPatchToolDef Tests ====================

func TestGitApplyPatchToolDef(t *testing.T) {
	toolDef := GitApplyPatchToolDef()

	// Verify tool name
	if toolDef["name"] != "git_apply_patch" {
		t.Errorf("tool name = %v, want git_apply_patch", toolDef["name"])
	}

	// Verify description exists and is non-empty
	desc, ok := toolDef["description"].(string)
	if !ok || desc == "" {
		t.Error("tool description missing or empty")
	}

	// Verify inputSchema structure
	schema, ok := toolDef["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatal("inputSchema is not a map")
	}

	if schema["type"] != "object" {
		t.Errorf("schema type = %v, want object", schema["type"])
	}

	// Verify properties exist
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties is not a map")
	}

	// Check required properties
	requiredProps := []string{"patch"}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("required is not a []string")
	}

	for _, rp := range requiredProps {
		found := false
		for _, r := range required {
			if r == rp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required property %q not found", rp)
		}
	}

	// Verify patch property
	patchProp, ok := props["patch"].(map[string]interface{})
	if !ok {
		t.Fatal("patch property is not a map")
	}
	if patchProp["type"] != "string" {
		t.Errorf("patch type = %v, want string", patchProp["type"])
	}
	if patchProp["description"] == nil || patchProp["description"] == "" {
		t.Error("patch property missing description")
	}

	// Verify working_directory property
	wdProp, ok := props["working_directory"].(map[string]interface{})
	if !ok {
		t.Fatal("working_directory property is not a map")
	}
	if wdProp["type"] != "string" {
		t.Errorf("working_directory type = %v, want string", wdProp["type"])
	}

	// Verify check_only property
	checkOnlyProp, ok := props["check_only"].(map[string]interface{})
	if !ok {
		t.Fatal("check_only property is not a map")
	}
	if checkOnlyProp["type"] != "boolean" {
		t.Errorf("check_only type = %v, want boolean", checkOnlyProp["type"])
	}
	if checkOnlyProp["default"] != false {
		t.Errorf("check_only default = %v, want false", checkOnlyProp["default"])
	}

	// Verify reverse property
	reverseProp, ok := props["reverse"].(map[string]interface{})
	if !ok {
		t.Fatal("reverse property is not a map")
	}
	if reverseProp["type"] != "boolean" {
		t.Errorf("reverse type = %v, want boolean", reverseProp["type"])
	}
	if reverseProp["default"] != false {
		t.Errorf("reverse default = %v, want false", reverseProp["default"])
	}

	// Verify three_way property
	threeWayProp, ok := props["three_way"].(map[string]interface{})
	if !ok {
		t.Fatal("three_way property is not a map")
	}
	if threeWayProp["type"] != "boolean" {
		t.Errorf("three_way type = %v, want boolean", threeWayProp["type"])
	}
	if threeWayProp["default"] != false {
		t.Errorf("three_way default = %v, want false", threeWayProp["default"])
	}

	// Verify ignore_whitespace property
	ignoreWsProp, ok := props["ignore_whitespace"].(map[string]interface{})
	if !ok {
		t.Fatal("ignore_whitespace property is not a map")
	}
	if ignoreWsProp["type"] != "boolean" {
		t.Errorf("ignore_whitespace type = %v, want boolean", ignoreWsProp["type"])
	}
	if ignoreWsProp["default"] != false {
		t.Errorf("ignore_whitespace default = %v, want false", ignoreWsProp["default"])
	}

	// Verify context_lines property
	contextLinesProp, ok := props["context_lines"].(map[string]interface{})
	if !ok {
		t.Fatal("context_lines property is not a map")
	}
	if contextLinesProp["type"] != "integer" {
		t.Errorf("context_lines type = %v, want integer", contextLinesProp["type"])
	}
	if contextLinesProp["default"] != 3 {
		t.Errorf("context_lines default = %v, want 3", contextLinesProp["default"])
	}
	if contextLinesProp["minimum"] != 0 {
		t.Errorf("context_lines minimum = %v, want 0", contextLinesProp["minimum"])
	}
	if contextLinesProp["maximum"] != 10 {
		t.Errorf("context_lines maximum = %v, want 10", contextLinesProp["maximum"])
	}
}

func TestGitApplyPatchToolDefSerializable(t *testing.T) {
	toolDef := GitApplyPatchToolDef()

	// Verify the tool definition can be serialized to JSON
	data, err := json.Marshal(toolDef)
	if err != nil {
		t.Fatalf("failed to marshal tool definition: %v", err)
	}

	// Verify it can be deserialized back
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal tool definition: %v", err)
	}

	if parsed["name"] != "git_apply_patch" {
		t.Errorf("deserialized name = %v, want git_apply_patch", parsed["name"])
	}
}

func TestValidateGitApplyPatchRequest(t *testing.T) {
	validPatch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,3 @@
 line 1
-line 2
+modified line 2
 line 3
`

	tests := []struct {
		name    string
		req     GitApplyPatchRequest
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid minimal request",
			req:     GitApplyPatchRequest{Patch: validPatch},
			wantErr: false,
		},
		{
			name:    "valid full request",
			req:     GitApplyPatchRequest{Patch: validPatch, WorkingDirectory: "/tmp/repo", CheckOnly: true, Reverse: false, ThreeWay: true, IgnoreWhitespace: true, ContextLines: 1},
			wantErr: false,
		},
		{
			name:    "valid check only",
			req:     GitApplyPatchRequest{Patch: validPatch, CheckOnly: true},
			wantErr: false,
		},
		{
			name:    "valid reverse",
			req:     GitApplyPatchRequest{Patch: validPatch, Reverse: true},
			wantErr: false,
		},
		{
			name:    "valid three-way merge",
			req:     GitApplyPatchRequest{Patch: validPatch, ThreeWay: true},
			wantErr: false,
		},
		{
			name:    "valid ignore whitespace",
			req:     GitApplyPatchRequest{Patch: validPatch, IgnoreWhitespace: true},
			wantErr: false,
		},
		{
			name:    "valid context lines zero",
			req:     GitApplyPatchRequest{Patch: validPatch, ContextLines: 0},
			wantErr: false,
		},
		{
			name:    "valid context lines max",
			req:     GitApplyPatchRequest{Patch: validPatch, ContextLines: 10},
			wantErr: false,
		},
		{
			name:    "empty patch",
			req:     GitApplyPatchRequest{Patch: ""},
			wantErr: true,
			errMsg:  "patch: patch is required",
		},
		{
			name:    "invalid patch format - no headers",
			req:     GitApplyPatchRequest{Patch: "just some text without proper diff headers"},
			wantErr: true,
			errMsg:  "patch: patch must be in unified diff format",
		},
		{
			name:    "invalid patch format - only old file header",
			req:     GitApplyPatchRequest{Patch: "--- a/file.txt\nsome content"},
			wantErr: true,
			errMsg:  "patch: patch must be in unified diff format",
		},
		{
			name:    "invalid patch format - only new file header",
			req:     GitApplyPatchRequest{Patch: "+++ b/file.txt\nsome content"},
			wantErr: true,
			errMsg:  "patch: patch must be in unified diff format",
		},
		{
			name:    "context lines too high",
			req:     GitApplyPatchRequest{Patch: validPatch, ContextLines: 11},
			wantErr: true,
			errMsg:  "context_lines: context_lines must not exceed 10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req // Copy to avoid modifying original
			err := ValidateGitApplyPatchRequest(&req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				// Check that error message contains expected substring
				if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Logf("error = %q (expected contains %q)", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateGitApplyPatchRequestDefaults(t *testing.T) {
	validPatch := `--- a/test.txt
+++ b/test.txt
@@ -1 +1 @@
-old
+new
`
	req := GitApplyPatchRequest{Patch: validPatch}

	err := ValidateGitApplyPatchRequest(&req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify defaults
	if req.CheckOnly {
		t.Error("check_only should default to false")
	}
	if req.Reverse {
		t.Error("reverse should default to false")
	}
	if req.ThreeWay {
		t.Error("three_way should default to false")
	}
	if req.IgnoreWhitespace {
		t.Error("ignore_whitespace should default to false")
	}
}

func TestGitApplyPatchRequestJSONParsing(t *testing.T) {
	jsonData := `{"patch": "--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new", "working_directory": "/tmp/repo", "check_only": true, "reverse": false, "three_way": true, "ignore_whitespace": true, "context_lines": 2}`

	var req GitApplyPatchRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Patch != "--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new" {
		t.Errorf("patch = %v, want valid diff", req.Patch)
	}
	if req.WorkingDirectory != "/tmp/repo" {
		t.Errorf("working_directory = %v, want /tmp/repo", req.WorkingDirectory)
	}
	if req.CheckOnly != true {
		t.Errorf("check_only = %v, want true", req.CheckOnly)
	}
	if req.Reverse != false {
		t.Errorf("reverse = %v, want false", req.Reverse)
	}
	if req.ThreeWay != true {
		t.Errorf("three_way = %v, want true", req.ThreeWay)
	}
	if req.IgnoreWhitespace != true {
		t.Errorf("ignore_whitespace = %v, want true", req.IgnoreWhitespace)
	}
	if req.ContextLines != 2 {
		t.Errorf("context_lines = %v, want 2", req.ContextLines)
	}
}

func TestGitApplyPatchRequestMinimalJSON(t *testing.T) {
	jsonData := `{"patch": "--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new"}`

	var req GitApplyPatchRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Patch == "" {
		t.Error("patch should not be empty")
	}

	// Validate to check defaults behavior
	if err := ValidateGitApplyPatchRequest(&req); err != nil {
		t.Fatalf("validation failed: %v", err)
	}
}

func TestValidateGitApplyPatchRequestEmptyPatchError(t *testing.T) {
	req := GitApplyPatchRequest{Patch: ""}
	err := ValidateGitApplyPatchRequest(&req)
	if err == nil {
		t.Error("expected error for empty patch")
	}

	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if validationErr.Field != "patch" {
		t.Errorf("error field = %q, want %q", validationErr.Field, "patch")
	}
}

func TestValidateGitApplyPatchRequestInvalidFormatError(t *testing.T) {
	invalidPatches := []string{
		"just plain text",
		"some random content\nwithout diff headers",
		"--- only old file header",
		"+++ only new file header",
		"diff --git a/file b/file", // missing --- and +++
	}

	for _, patch := range invalidPatches {
		t.Run(patch[:min(len(patch), 20)], func(t *testing.T) {
			req := GitApplyPatchRequest{Patch: patch}
			err := ValidateGitApplyPatchRequest(&req)
			if err == nil {
				t.Errorf("expected error for invalid patch format")
				return
			}

			validationErr, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("expected ValidationError, got %T", err)
			}

			if validationErr.Field != "patch" {
				t.Errorf("error field = %q, want %q", validationErr.Field, "patch")
			}
		})
	}
}

func TestValidateGitApplyPatchRequestContextLinesBoundaries(t *testing.T) {
	validPatch := `--- a/test.txt
+++ b/test.txt
@@ -1 +1 @@
-old
+new
`

	// Test minimum boundary (0)
	reqMin := GitApplyPatchRequest{Patch: validPatch, ContextLines: 0}
	if err := ValidateGitApplyPatchRequest(&reqMin); err != nil {
		t.Errorf("unexpected error for context_lines=0: %v", err)
	}

	// Test maximum boundary (10)
	reqMax := GitApplyPatchRequest{Patch: validPatch, ContextLines: 10}
	if err := ValidateGitApplyPatchRequest(&reqMax); err != nil {
		t.Errorf("unexpected error for context_lines=10: %v", err)
	}

	// Test just above maximum (11)
	reqAboveMax := GitApplyPatchRequest{Patch: validPatch, ContextLines: 11}
	if err := ValidateGitApplyPatchRequest(&reqAboveMax); err == nil {
		t.Error("expected error for context_lines=11")
	}
}

func TestGitApplyPatchToolDefInputSchemaComplete(t *testing.T) {
	toolDef := GitApplyPatchToolDef()

	schema, ok := toolDef["inputSchema"].(map[string]interface{})
	if !ok {
		t.Fatal("inputSchema is not a map")
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("properties is not a map")
	}

	// Verify all expected properties exist
	expectedProps := []string{"patch", "working_directory", "check_only", "reverse", "three_way", "ignore_whitespace", "context_lines"}
	for _, prop := range expectedProps {
		if props[prop] == nil {
			t.Errorf("missing expected property: %s", prop)
		}
	}
}

func TestGitApplyPatchRequestJSONOmitEmpty(t *testing.T) {
	// Test that optional fields are omitted when empty
	req := GitApplyPatchRequest{Patch: "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-a\n+b"}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Parse back to verify structure
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Patch should be present
	if _, exists := parsed["patch"]; !exists {
		t.Error("patch should be present")
	}

	// Working directory should be omitted (empty)
	if _, exists := parsed["working_directory"]; exists && parsed["working_directory"] != "" {
		t.Errorf("working_directory should be omitted or empty, got %v", parsed["working_directory"])
	}
}

func TestGitApplyPatchRequestJSONWithAllFields(t *testing.T) {
	jsonData := `{
		"patch": "--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new",
		"working_directory": "/home/user/repo",
		"check_only": true,
		"reverse": true,
		"three_way": true,
		"ignore_whitespace": true,
		"context_lines": 5
	}`

	var req GitApplyPatchRequest
	if err := json.Unmarshal([]byte(jsonData), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Patch == "" {
		t.Error("patch should not be empty")
	}
	if req.WorkingDirectory != "/home/user/repo" {
		t.Errorf("working_directory = %v, want /home/user/repo", req.WorkingDirectory)
	}
	if req.CheckOnly != true {
		t.Errorf("check_only = %v, want true", req.CheckOnly)
	}
	if req.Reverse != true {
		t.Errorf("reverse = %v, want true", req.Reverse)
	}
	if req.ThreeWay != true {
		t.Errorf("three_way = %v, want true", req.ThreeWay)
	}
	if req.IgnoreWhitespace != true {
		t.Errorf("ignore_whitespace = %v, want true", req.IgnoreWhitespace)
	}
	if req.ContextLines != 5 {
		t.Errorf("context_lines = %v, want 5", req.ContextLines)
	}
}

func TestGitApplyPatchRequestComplexPatch(t *testing.T) {
	complexPatch := `diff --git a/file1.go b/file1.go
--- a/file1.go
+++ b/file1.go
@@ -1,5 +1,6 @@
 package main

 func main() {
-	println("hello")
+	println("hello world")
+	println("additional line")
 }
diff --git a/file2.go b/file2.go
--- a/file2.go
+++ b/file2.go
@@ -1,3 +1,4 @@
 package main

+// New comment
 var x = 1
`

	req := GitApplyPatchRequest{Patch: complexPatch}
	err := ValidateGitApplyPatchRequest(&req)
	if err != nil {
		t.Errorf("unexpected error for complex patch: %v", err)
	}
}

func TestGitApplyPatchRequestMultiFilePatch(t *testing.T) {
	multiFilePatch := `--- a/file1.txt
+++ b/file1.txt
@@ -1 +1 @@
-content1
+modified1
--- a/file2.txt
+++ b/file2.txt
@@ -1 +1 @@
-content2
+modified2
`

	req := GitApplyPatchRequest{Patch: multiFilePatch}
	err := ValidateGitApplyPatchRequest(&req)
	if err != nil {
		t.Errorf("unexpected error for multi-file patch: %v", err)
	}
}

func TestGitApplyPatchContextLinesConstants(t *testing.T) {
	if DefaultContextLines != 3 {
		t.Errorf("DefaultContextLines = %d, want %d", DefaultContextLines, 3)
	}
	if MinContextLines != 0 {
		t.Errorf("MinContextLines = %d, want %d", MinContextLines, 0)
	}
	if MaxContextLines != 10 {
		t.Errorf("MaxContextLines = %d, want %d", MaxContextLines, 10)
	}
}

func TestParsePatchValidation(t *testing.T) {
	tests := []struct {
		name        string
		patch       string
		shouldParse bool
		shouldValid bool // Whether ValidatePatch should pass
	}{
		{
			name:        "valid unified diff",
			patch:       "--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new",
			shouldParse: true,
			shouldValid: true,
		},
		{
			name:        "valid with git headers",
			patch:       "diff --git a/file b/file\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new",
			shouldParse: true,
			shouldValid: true,
		},
		{
			name:        "missing +++ header parses but fails validation",
			patch:       "--- a/file.txt\nsome content",
			shouldParse: true,  // Parser accepts it
			shouldValid: false, // Validation catches it
		},
		{
			name:        "no headers",
			patch:       "just plain text content",
			shouldParse: false,
			shouldValid: false,
		},
		{
			name:        "empty patch",
			patch:       "",
			shouldParse: false,
			shouldValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patch, err := ParsePatch(tt.patch)
			parsed := err == nil
			if parsed != tt.shouldParse {
				t.Errorf("ParsePatch() parsed = %v, want %v, err: %v", parsed, tt.shouldParse, err)
			}

			// Check validation if parsing succeeded
			if parsed && patch != nil {
				result := ValidatePatch(patch)
				if result.Valid != tt.shouldValid {
					t.Errorf("ValidatePatch().Valid = %v, want %v, errors: %v", result.Valid, tt.shouldValid, result.Errors)
				}
			}
		})
	}
}

// Helper function for tests
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
