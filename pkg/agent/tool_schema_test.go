package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// Compile-time verification that BaseToolSchemaTranslator implements ToolSchemaTranslator
var _ ToolSchemaTranslator = (*BaseToolSchemaTranslator)(nil)

func TestParseJSONSchema(t *testing.T) {
	t.Run("valid object schema", func(t *testing.T) {
		data := json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {"type": "string", "description": "File path"},
				"content": {"type": "string"}
			},
			"required": ["path"]
		}`)

		schema, err := ParseJSONSchema(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if schema.Type != JSONSchemaTypeObject {
			t.Errorf("expected type %q, got %q", JSONSchemaTypeObject, schema.Type)
		}
		if len(schema.Properties) != 2 {
			t.Errorf("expected 2 properties, got %d", len(schema.Properties))
		}
		if len(schema.Required) != 1 || schema.Required[0] != "path" {
			t.Errorf("expected required=[path], got %v", schema.Required)
		}
	})

	t.Run("empty data", func(t *testing.T) {
		_, err := ParseJSONSchema(json.RawMessage{})
		if !errors.Is(err, ErrInvalidInputSchema) {
			t.Errorf("expected ErrInvalidInputSchema, got %v", err)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := ParseJSONSchema(json.RawMessage(`{invalid`))
		if !errors.Is(err, ErrInvalidInputSchema) {
			t.Errorf("expected ErrInvalidInputSchema, got %v", err)
		}
	})

	t.Run("schema with enum", func(t *testing.T) {
		data := json.RawMessage(`{
			"type": "string",
			"enum": ["option1", "option2", "option3"]
		}`)

		schema, err := ParseJSONSchema(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(schema.Enum) != 3 {
			t.Errorf("expected 3 enum values, got %d", len(schema.Enum))
		}
	})

	t.Run("schema with array", func(t *testing.T) {
		data := json.RawMessage(`{
			"type": "array",
			"items": {"type": "string"},
			"minItems": 1,
			"maxItems": 10
		}`)

		schema, err := ParseJSONSchema(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if schema.Type != JSONSchemaTypeArray {
			t.Errorf("expected type array, got %s", schema.Type)
		}
		if schema.Items == nil {
			t.Error("expected items to be set")
		}
		if *schema.MinItems != 1 {
			t.Errorf("expected minItems 1, got %d", *schema.MinItems)
		}
	})

	t.Run("schema with number constraints", func(t *testing.T) {
		data := json.RawMessage(`{
			"type": "number",
			"minimum": 0,
			"maximum": 100,
			"multipleOf": 0.5
		}`)

		schema, err := ParseJSONSchema(data)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if *schema.Minimum != 0 {
			t.Errorf("expected minimum 0, got %f", *schema.Minimum)
		}
		if *schema.Maximum != 100 {
			t.Errorf("expected maximum 100, got %f", *schema.Maximum)
		}
	})
}

func TestJSONSchemaToJSON(t *testing.T) {
	schema := &JSONSchema{
		Type: JSONSchemaTypeObject,
		Properties: map[string]*JSONSchema{
			"name": {Type: JSONSchemaTypeString, Description: "User name"},
		},
		Required: []string{"name"},
	}

	data, err := schema.ToJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Parse back and verify
	parsed, err := ParseJSONSchema(data)
	if err != nil {
		t.Fatalf("failed to parse generated JSON: %v", err)
	}

	if parsed.Type != JSONSchemaTypeObject {
		t.Errorf("expected type object, got %s", parsed.Type)
	}
}

func TestJSONSchemaValidate(t *testing.T) {
	t.Run("valid object schema", func(t *testing.T) {
		schema := &JSONSchema{
			Type: JSONSchemaTypeObject,
			Properties: map[string]*JSONSchema{
				"path": {Type: JSONSchemaTypeString},
			},
		}

		if err := schema.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("non-object root without special fields", func(t *testing.T) {
		schema := &JSONSchema{
			Type: JSONSchemaTypeString,
		}

		err := schema.Validate()
		if !errors.Is(err, ErrUnsupportedSchemaType) {
			t.Errorf("expected ErrUnsupportedSchemaType, got %v", err)
		}
	})

	t.Run("allows $ref", func(t *testing.T) {
		schema := &JSONSchema{
			Ref: "#/definitions/SomeType",
		}

		if err := schema.Validate(); err != nil {
			t.Errorf("unexpected error for $ref schema: %v", err)
		}
	})

	t.Run("allows anyOf", func(t *testing.T) {
		schema := &JSONSchema{
			AnyOf: []*JSONSchema{
				{Type: JSONSchemaTypeString},
				{Type: JSONSchemaTypeNumber},
			},
		}

		if err := schema.Validate(); err != nil {
			t.Errorf("unexpected error for anyOf schema: %v", err)
		}
	})

	t.Run("nil property", func(t *testing.T) {
		schema := &JSONSchema{
			Type: JSONSchemaTypeObject,
			Properties: map[string]*JSONSchema{
				"valid": {Type: JSONSchemaTypeString},
				"nil":   nil,
			},
		}

		err := schema.Validate()
		if !errors.Is(err, ErrInvalidInputSchema) {
			t.Errorf("expected ErrInvalidInputSchema, got %v", err)
		}
	})
}

func TestJSONSchemaIsRequired(t *testing.T) {
	schema := &JSONSchema{
		Required: []string{"name", "path"},
	}

	if !schema.IsRequired("name") {
		t.Error("expected 'name' to be required")
	}
	if !schema.IsRequired("path") {
		t.Error("expected 'path' to be required")
	}
	if schema.IsRequired("optional") {
		t.Error("expected 'optional' to not be required")
	}
}

func TestJSONSchemaGetPropertyNames(t *testing.T) {
	schema := &JSONSchema{
		Properties: map[string]*JSONSchema{
			"name":    {Type: JSONSchemaTypeString},
			"age":     {Type: JSONSchemaTypeInteger},
			"enabled": {Type: JSONSchemaTypeBoolean},
		},
	}

	names := schema.GetPropertyNames()
	if len(names) != 3 {
		t.Errorf("expected 3 property names, got %d", len(names))
	}

	nameMap := make(map[string]bool)
	for _, n := range names {
		nameMap[n] = true
	}

	for _, expected := range []string{"name", "age", "enabled"} {
		if !nameMap[expected] {
			t.Errorf("expected property %q not found", expected)
		}
	}
}

func TestBaseToolSchemaTranslator_ValidateToolDefinition(t *testing.T) {
	translator := NewBaseToolSchemaTranslator("test")

	t.Run("valid tool", func(t *testing.T) {
		tool := ToolDefinition{
			Name:        "read_file",
			Description: "Read a file",
			InputSchema: json.RawMessage(`{"type": "object", "properties": {"path": {"type": "string"}}}`),
		}

		if err := translator.ValidateToolDefinition(tool); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		tool := ToolDefinition{
			Description: "Some tool",
		}

		err := translator.ValidateToolDefinition(tool)
		if !errors.Is(err, ErrMissingToolName) {
			t.Errorf("expected ErrMissingToolName, got %v", err)
		}
	})

	t.Run("invalid schema", func(t *testing.T) {
		tool := ToolDefinition{
			Name:        "bad_tool",
			InputSchema: json.RawMessage(`{invalid json`),
		}

		err := translator.ValidateToolDefinition(tool)
		if !errors.Is(err, ErrInvalidToolDefinition) {
			t.Errorf("expected ErrInvalidToolDefinition, got %v", err)
		}
	})

	t.Run("tool without schema", func(t *testing.T) {
		tool := ToolDefinition{
			Name:        "simple_tool",
			Description: "A tool without parameters",
		}

		if err := translator.ValidateToolDefinition(tool); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestBaseToolSchemaTranslator_TranslateToProvider(t *testing.T) {
	translator := NewBaseToolSchemaTranslator("test")

	t.Run("basic translation", func(t *testing.T) {
		tool := ToolDefinition{
			Name:        "write_file",
			Description: "Write content to a file",
			InputSchema: json.RawMessage(`{"type": "object", "properties": {"path": {"type": "string"}, "content": {"type": "string"}}}`),
		}

		result, err := translator.TranslateToProvider(tool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("expected map, got %T", result)
		}

		if resultMap["name"] != "write_file" {
			t.Errorf("expected name 'write_file', got %v", resultMap["name"])
		}
		if resultMap["description"] != "Write content to a file" {
			t.Errorf("expected description, got %v", resultMap["description"])
		}
		if resultMap["parameters"] == nil {
			t.Error("expected parameters to be set")
		}
	})

	t.Run("invalid tool", func(t *testing.T) {
		tool := ToolDefinition{Name: ""}
		_, err := translator.TranslateToProvider(tool)
		if err == nil {
			t.Error("expected error for invalid tool")
		}
	})
}

func TestBaseToolSchemaTranslator_TranslateFromProvider(t *testing.T) {
	translator := NewBaseToolSchemaTranslator("test")

	t.Run("map with id, name, arguments", func(t *testing.T) {
		providerCall := map[string]interface{}{
			"id":        "call-123",
			"name":      "read_file",
			"arguments": `{"path": "/tmp/test.txt"}`,
		}

		block, err := translator.TranslateFromProvider(providerCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.Type != ContentTypeToolUse {
			t.Errorf("expected type tool_use, got %s", block.Type)
		}
		if block.ToolUseID != "call-123" {
			t.Errorf("expected ID 'call-123', got %s", block.ToolUseID)
		}
		if block.ToolName != "read_file" {
			t.Errorf("expected name 'read_file', got %s", block.ToolName)
		}
	})

	t.Run("map with input instead of arguments", func(t *testing.T) {
		providerCall := map[string]interface{}{
			"id":    "call-456",
			"name":  "write_file",
			"input": map[string]interface{}{"path": "/tmp/out.txt", "content": "hello"},
		}

		block, err := translator.TranslateFromProvider(providerCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.ToolName != "write_file" {
			t.Errorf("expected name 'write_file', got %s", block.ToolName)
		}

		var input map[string]interface{}
		if err := json.Unmarshal(block.ToolInput, &input); err != nil {
			t.Fatalf("failed to parse tool input: %v", err)
		}
		if input["path"] != "/tmp/out.txt" {
			t.Errorf("expected path '/tmp/out.txt', got %v", input["path"])
		}
	})

	t.Run("fallback to function field", func(t *testing.T) {
		providerCall := map[string]interface{}{
			"id":       "call-789",
			"function": "run_command",
		}

		block, err := translator.TranslateFromProvider(providerCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.ToolName != "run_command" {
			t.Errorf("expected name 'run_command', got %s", block.ToolName)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		providerCall := map[string]interface{}{
			"id": "call-000",
		}

		_, err := translator.TranslateFromProvider(providerCall)
		if !errors.Is(err, ErrInvalidToolCall) {
			t.Errorf("expected ErrInvalidToolCall, got %v", err)
		}
	})

	t.Run("non-map input", func(t *testing.T) {
		_, err := translator.TranslateFromProvider("not a map")
		if !errors.Is(err, ErrInvalidToolCall) {
			t.Errorf("expected ErrInvalidToolCall, got %v", err)
		}
	})
}

func TestBaseToolSchemaTranslator_TranslateToolResult(t *testing.T) {
	translator := NewBaseToolSchemaTranslator("test")

	t.Run("success result", func(t *testing.T) {
		result := ContentBlock{
			Type:         ContentTypeToolResult,
			ToolResultID: "call-123",
			ToolOutput:   "file contents here",
		}

		output, err := translator.TranslateToolResult(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		outputMap, ok := output.(map[string]interface{})
		if !ok {
			t.Fatalf("expected map, got %T", output)
		}

		if outputMap["tool_use_id"] != "call-123" {
			t.Errorf("expected tool_use_id 'call-123', got %v", outputMap["tool_use_id"])
		}
		if outputMap["content"] != "file contents here" {
			t.Errorf("expected content, got %v", outputMap["content"])
		}
		if _, exists := outputMap["is_error"]; exists {
			t.Error("did not expect is_error for success result")
		}
	})

	t.Run("error result", func(t *testing.T) {
		result := ContentBlock{
			Type:         ContentTypeToolResult,
			ToolResultID: "call-456",
			ToolOutput:   "",
			ToolError:    "file not found",
		}

		output, err := translator.TranslateToolResult(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		outputMap := output.(map[string]interface{})
		if outputMap["is_error"] != true {
			t.Error("expected is_error to be true")
		}
		if outputMap["error"] != "file not found" {
			t.Errorf("expected error message, got %v", outputMap["error"])
		}
	})

	t.Run("wrong content type", func(t *testing.T) {
		result := ContentBlock{
			Type: ContentTypeText,
			Text: "not a tool result",
		}

		_, err := translator.TranslateToolResult(result)
		if err == nil {
			t.Error("expected error for wrong content type")
		}
	})

	t.Run("missing tool ID", func(t *testing.T) {
		result := ContentBlock{
			Type:       ContentTypeToolResult,
			ToolOutput: "output",
		}

		_, err := translator.TranslateToolResult(result)
		if !errors.Is(err, ErrMissingToolID) {
			t.Errorf("expected ErrMissingToolID, got %v", err)
		}
	})
}

func TestToolCallExtractor(t *testing.T) {
	extractor := NewToolCallExtractor()

	content := []ContentBlock{
		{Type: ContentTypeText, Text: "Let me help you."},
		{Type: ContentTypeToolUse, ToolUseID: "call-1", ToolName: "read_file"},
		{Type: ContentTypeToolUse, ToolUseID: "call-2", ToolName: "write_file"},
		{Type: ContentTypeToolUse, ToolUseID: "call-3", ToolName: "read_file"},
	}

	t.Run("ExtractToolCalls", func(t *testing.T) {
		calls := extractor.ExtractToolCalls(content)
		if len(calls) != 3 {
			t.Errorf("expected 3 tool calls, got %d", len(calls))
		}
	})

	t.Run("ExtractToolCallByID", func(t *testing.T) {
		call := extractor.ExtractToolCallByID(content, "call-2")
		if call == nil {
			t.Fatal("expected to find call-2")
		}
		if call.ToolName != "write_file" {
			t.Errorf("expected name 'write_file', got %s", call.ToolName)
		}

		notFound := extractor.ExtractToolCallByID(content, "call-999")
		if notFound != nil {
			t.Error("expected nil for non-existent ID")
		}
	})

	t.Run("ExtractToolCallsByName", func(t *testing.T) {
		readCalls := extractor.ExtractToolCallsByName(content, "read_file")
		if len(readCalls) != 2 {
			t.Errorf("expected 2 read_file calls, got %d", len(readCalls))
		}

		writeCalls := extractor.ExtractToolCallsByName(content, "write_file")
		if len(writeCalls) != 1 {
			t.Errorf("expected 1 write_file call, got %d", len(writeCalls))
		}

		noCalls := extractor.ExtractToolCallsByName(content, "delete_file")
		if len(noCalls) != 0 {
			t.Errorf("expected 0 calls, got %d", len(noCalls))
		}
	})
}

func TestToolInputParser(t *testing.T) {
	parser := NewToolInputParser()

	input := json.RawMessage(`{
		"path": "/tmp/test.txt",
		"count": 42,
		"enabled": true,
		"nested": {"key": "value"}
	}`)

	t.Run("ParseAsMap", func(t *testing.T) {
		m, err := parser.ParseAsMap(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(m) != 4 {
			t.Errorf("expected 4 keys, got %d", len(m))
		}
	})

	t.Run("ParseAsMap empty", func(t *testing.T) {
		m, err := parser.ParseAsMap(json.RawMessage{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(m) != 0 {
			t.Errorf("expected empty map, got %d keys", len(m))
		}
	})

	t.Run("ParseString", func(t *testing.T) {
		val, err := parser.ParseString(input, "path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "/tmp/test.txt" {
			t.Errorf("expected '/tmp/test.txt', got %s", val)
		}
	})

	t.Run("ParseString not found", func(t *testing.T) {
		_, err := parser.ParseString(input, "nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent key")
		}
	})

	t.Run("ParseString wrong type", func(t *testing.T) {
		_, err := parser.ParseString(input, "count")
		if err == nil {
			t.Error("expected error for wrong type")
		}
	})

	t.Run("ParseInt", func(t *testing.T) {
		val, err := parser.ParseInt(input, "count")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != 42 {
			t.Errorf("expected 42, got %d", val)
		}
	})

	t.Run("ParseBool", func(t *testing.T) {
		val, err := parser.ParseBool(input, "enabled")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !val {
			t.Error("expected true")
		}
	})

	t.Run("HasKey", func(t *testing.T) {
		if !parser.HasKey(input, "path") {
			t.Error("expected HasKey to return true for 'path'")
		}
		if parser.HasKey(input, "missing") {
			t.Error("expected HasKey to return false for 'missing'")
		}
	})
}

func TestToolResultBuilder(t *testing.T) {
	t.Run("success result", func(t *testing.T) {
		result := NewToolResultBuilder("call-123").
			WithOutput("file contents").
			Build()

		if result.Type != ContentTypeToolResult {
			t.Errorf("expected type tool_result, got %s", result.Type)
		}
		if result.ToolResultID != "call-123" {
			t.Errorf("expected ID 'call-123', got %s", result.ToolResultID)
		}
		if result.ToolOutput != "file contents" {
			t.Errorf("expected output 'file contents', got %s", result.ToolOutput)
		}
		if result.ToolError != "" {
			t.Errorf("expected no error, got %s", result.ToolError)
		}
	})

	t.Run("JSON output", func(t *testing.T) {
		data := map[string]interface{}{
			"files": []string{"a.txt", "b.txt"},
			"count": 2,
		}

		result := NewToolResultBuilder("call-456").
			WithJSONOutput(data).
			Build()

		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(result.ToolOutput), &parsed); err != nil {
			t.Fatalf("failed to parse JSON output: %v", err)
		}
		if parsed["count"].(float64) != 2 {
			t.Errorf("expected count 2, got %v", parsed["count"])
		}
	})

	t.Run("error result", func(t *testing.T) {
		result := NewToolResultBuilder("call-789").
			WithError(errors.New("permission denied")).
			Build()

		if result.ToolError != "permission denied" {
			t.Errorf("expected error 'permission denied', got %s", result.ToolError)
		}
	})

	t.Run("build message", func(t *testing.T) {
		msg := NewToolResultBuilder("call-abc").
			WithOutput("done").
			BuildMessage()

		if msg.Role != RoleTool {
			t.Errorf("expected role tool, got %s", msg.Role)
		}
		if len(msg.Content) != 1 {
			t.Fatalf("expected 1 content block, got %d", len(msg.Content))
		}
		if msg.Content[0].ToolResultID != "call-abc" {
			t.Errorf("expected ID 'call-abc', got %s", msg.Content[0].ToolResultID)
		}
	})
}

// Compile-time verification that OpenAIToolSchemaTranslator implements ToolSchemaTranslator
var _ ToolSchemaTranslator = (*OpenAIToolSchemaTranslator)(nil)

func TestOpenAIToolSchemaTranslator_TranslateToProvider(t *testing.T) {
	translator := NewOpenAIToolSchemaTranslator()

	t.Run("basic tool translation", func(t *testing.T) {
		tool := ToolDefinition{
			Name:        "read_file",
			Description: "Read contents of a file",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File path"}},"required":["path"]}`),
		}

		result, err := translator.TranslateToProvider(tool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		openAITool, ok := result.(OpenAIFunctionTool)
		if !ok {
			t.Fatalf("expected OpenAIFunctionTool, got %T", result)
		}

		if openAITool.Type != "function" {
			t.Errorf("expected type 'function', got %q", openAITool.Type)
		}
		if openAITool.Function.Name != "read_file" {
			t.Errorf("expected name 'read_file', got %q", openAITool.Function.Name)
		}
		if openAITool.Function.Description != "Read contents of a file" {
			t.Errorf("expected description, got %q", openAITool.Function.Description)
		}

		// Verify parameters are preserved
		var params map[string]interface{}
		if err := json.Unmarshal(openAITool.Function.Parameters, &params); err != nil {
			t.Fatalf("failed to parse parameters: %v", err)
		}
		if params["type"] != "object" {
			t.Errorf("expected type 'object', got %v", params["type"])
		}
	})

	t.Run("tool without parameters gets empty object schema", func(t *testing.T) {
		tool := ToolDefinition{
			Name:        "get_time",
			Description: "Get current time",
		}

		result, err := translator.TranslateToProvider(tool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		openAITool := result.(OpenAIFunctionTool)

		var params map[string]interface{}
		if err := json.Unmarshal(openAITool.Function.Parameters, &params); err != nil {
			t.Fatalf("failed to parse parameters: %v", err)
		}
		if params["type"] != "object" {
			t.Errorf("expected empty object schema, got %v", params)
		}
	})

	t.Run("invalid tool (missing name)", func(t *testing.T) {
		tool := ToolDefinition{
			Description: "Tool without name",
		}

		_, err := translator.TranslateToProvider(tool)
		if !errors.Is(err, ErrMissingToolName) {
			t.Errorf("expected ErrMissingToolName, got %v", err)
		}
	})

	t.Run("tool with complex schema", func(t *testing.T) {
		tool := ToolDefinition{
			Name:        "search",
			Description: "Search for items",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string"},
					"filters": {
						"type": "array",
						"items": {"type": "string"}
					},
					"options": {
						"type": "object",
						"properties": {
							"limit": {"type": "integer", "minimum": 1, "maximum": 100}
						}
					}
				},
				"required": ["query"]
			}`),
		}

		result, err := translator.TranslateToProvider(tool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		openAITool := result.(OpenAIFunctionTool)

		var params map[string]interface{}
		if err := json.Unmarshal(openAITool.Function.Parameters, &params); err != nil {
			t.Fatalf("failed to parse parameters: %v", err)
		}

		props := params["properties"].(map[string]interface{})
		if _, ok := props["query"]; !ok {
			t.Error("expected query property")
		}
		if _, ok := props["filters"]; !ok {
			t.Error("expected filters property")
		}
		if _, ok := props["options"]; !ok {
			t.Error("expected options property")
		}
	})
}

func TestOpenAIToolSchemaTranslator_TranslateFromProvider(t *testing.T) {
	translator := NewOpenAIToolSchemaTranslator()

	t.Run("struct input", func(t *testing.T) {
		toolCall := OpenAIToolCallResponse{
			ID:   "call_abc123",
			Type: "function",
			Function: OpenAIToolCallFunction{
				Name:      "get_weather",
				Arguments: `{"location":"San Francisco","unit":"celsius"}`,
			},
		}

		block, err := translator.TranslateFromProvider(toolCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.Type != ContentTypeToolUse {
			t.Errorf("expected type tool_use, got %s", block.Type)
		}
		if block.ToolUseID != "call_abc123" {
			t.Errorf("expected ID 'call_abc123', got %s", block.ToolUseID)
		}
		if block.ToolName != "get_weather" {
			t.Errorf("expected name 'get_weather', got %s", block.ToolName)
		}

		var args map[string]interface{}
		if err := json.Unmarshal(block.ToolInput, &args); err != nil {
			t.Fatalf("failed to parse tool input: %v", err)
		}
		if args["location"] != "San Francisco" {
			t.Errorf("expected location 'San Francisco', got %v", args["location"])
		}
	})

	t.Run("pointer input", func(t *testing.T) {
		toolCall := &OpenAIToolCallResponse{
			ID:   "call_def456",
			Type: "function",
			Function: OpenAIToolCallFunction{
				Name:      "run_command",
				Arguments: `{"command":"ls -la"}`,
			},
		}

		block, err := translator.TranslateFromProvider(toolCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.ToolName != "run_command" {
			t.Errorf("expected name 'run_command', got %s", block.ToolName)
		}
	})

	t.Run("nil pointer", func(t *testing.T) {
		var toolCall *OpenAIToolCallResponse = nil

		_, err := translator.TranslateFromProvider(toolCall)
		if !errors.Is(err, ErrInvalidToolCall) {
			t.Errorf("expected ErrInvalidToolCall, got %v", err)
		}
	})

	t.Run("map input with nested function", func(t *testing.T) {
		toolCall := map[string]interface{}{
			"id":   "call_ghi789",
			"type": "function",
			"function": map[string]interface{}{
				"name":      "write_file",
				"arguments": `{"path":"/tmp/test.txt","content":"hello"}`,
			},
		}

		block, err := translator.TranslateFromProvider(toolCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.ToolUseID != "call_ghi789" {
			t.Errorf("expected ID 'call_ghi789', got %s", block.ToolUseID)
		}
		if block.ToolName != "write_file" {
			t.Errorf("expected name 'write_file', got %s", block.ToolName)
		}
	})

	t.Run("map input with map arguments", func(t *testing.T) {
		toolCall := map[string]interface{}{
			"id":   "call_jkl012",
			"type": "function",
			"function": map[string]interface{}{
				"name": "create_user",
				"arguments": map[string]interface{}{
					"name":  "John",
					"email": "john@example.com",
				},
			},
		}

		block, err := translator.TranslateFromProvider(toolCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var args map[string]interface{}
		if err := json.Unmarshal(block.ToolInput, &args); err != nil {
			t.Fatalf("failed to parse tool input: %v", err)
		}
		if args["name"] != "John" {
			t.Errorf("expected name 'John', got %v", args["name"])
		}
	})

	t.Run("map input with fallback fields (legacy)", func(t *testing.T) {
		toolCall := map[string]interface{}{
			"id":        "call_legacy",
			"name":      "legacy_tool",
			"arguments": `{"key":"value"}`,
		}

		block, err := translator.TranslateFromProvider(toolCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.ToolName != "legacy_tool" {
			t.Errorf("expected name 'legacy_tool', got %s", block.ToolName)
		}
	})

	t.Run("missing function name", func(t *testing.T) {
		toolCall := OpenAIToolCallResponse{
			ID:   "call_noname",
			Type: "function",
			Function: OpenAIToolCallFunction{
				Arguments: `{"x":1}`,
			},
		}

		_, err := translator.TranslateFromProvider(toolCall)
		if !errors.Is(err, ErrInvalidToolCall) {
			t.Errorf("expected ErrInvalidToolCall, got %v", err)
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		_, err := translator.TranslateFromProvider("not a valid type")
		if !errors.Is(err, ErrInvalidToolCall) {
			t.Errorf("expected ErrInvalidToolCall, got %v", err)
		}
	})
}

func TestOpenAIToolSchemaTranslator_TranslateToolResult(t *testing.T) {
	translator := NewOpenAIToolSchemaTranslator()

	t.Run("success result", func(t *testing.T) {
		result := ContentBlock{
			Type:         ContentTypeToolResult,
			ToolResultID: "call_abc123",
			ToolOutput:   `{"temperature": 72, "unit": "fahrenheit"}`,
		}

		output, err := translator.TranslateToolResult(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		msg, ok := output.(OpenAIToolResultMessage)
		if !ok {
			t.Fatalf("expected OpenAIToolResultMessage, got %T", output)
		}

		if msg.Role != "tool" {
			t.Errorf("expected role 'tool', got %q", msg.Role)
		}
		if msg.ToolCallID != "call_abc123" {
			t.Errorf("expected tool_call_id 'call_abc123', got %q", msg.ToolCallID)
		}
		if msg.Content != `{"temperature": 72, "unit": "fahrenheit"}` {
			t.Errorf("unexpected content: %q", msg.Content)
		}
	})

	t.Run("error result", func(t *testing.T) {
		result := ContentBlock{
			Type:         ContentTypeToolResult,
			ToolResultID: "call_def456",
			ToolError:    "file not found: /nonexistent.txt",
		}

		output, err := translator.TranslateToolResult(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		msg := output.(OpenAIToolResultMessage)
		if !strings.Contains(msg.Content, "Error:") {
			t.Errorf("expected error prefix in content, got %q", msg.Content)
		}
		if !strings.Contains(msg.Content, "file not found") {
			t.Errorf("expected error message in content, got %q", msg.Content)
		}
	})

	t.Run("wrong content type", func(t *testing.T) {
		result := ContentBlock{
			Type: ContentTypeText,
			Text: "not a tool result",
		}

		_, err := translator.TranslateToolResult(result)
		if err == nil {
			t.Error("expected error for wrong content type")
		}
	})

	t.Run("missing tool ID", func(t *testing.T) {
		result := ContentBlock{
			Type:       ContentTypeToolResult,
			ToolOutput: "some output",
		}

		_, err := translator.TranslateToolResult(result)
		if !errors.Is(err, ErrMissingToolID) {
			t.Errorf("expected ErrMissingToolID, got %v", err)
		}
	})
}

func TestOpenAIToolSchemaTranslator_TranslateToolChoice(t *testing.T) {
	translator := NewOpenAIToolSchemaTranslator()

	tests := []struct {
		input    string
		expected interface{}
	}{
		{"auto", "auto"},
		{"", "auto"},
		{"none", "none"},
		{"any", "required"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := translator.TranslateToolChoice(tc.input)
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}

	t.Run("specific tool name", func(t *testing.T) {
		result := translator.TranslateToolChoice("get_weather")

		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("expected map, got %T", result)
		}
		if resultMap["type"] != "function" {
			t.Errorf("expected type 'function', got %v", resultMap["type"])
		}

		funcMap, ok := resultMap["function"].(map[string]string)
		if !ok {
			t.Fatalf("expected function map, got %T", resultMap["function"])
		}
		if funcMap["name"] != "get_weather" {
			t.Errorf("expected name 'get_weather', got %v", funcMap["name"])
		}
	})
}

func TestOpenAIToolSchemaTranslator_TranslateMultipleTools(t *testing.T) {
	translator := NewOpenAIToolSchemaTranslator()

	tools := []ToolDefinition{
		{
			Name:        "read_file",
			Description: "Read a file",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		},
		{
			Name:        "write_file",
			Description: "Write a file",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}}}`),
		},
		{
			Name:        "list_files",
			Description: "List files in directory",
		},
	}

	result, err := translator.TranslateMultipleTools(tools)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("expected 3 tools, got %d", len(result))
	}

	// Verify each tool
	for i, translated := range result {
		openAITool, ok := translated.(OpenAIFunctionTool)
		if !ok {
			t.Errorf("tool %d: expected OpenAIFunctionTool, got %T", i, translated)
			continue
		}
		if openAITool.Type != "function" {
			t.Errorf("tool %d: expected type 'function', got %q", i, openAITool.Type)
		}
		if openAITool.Function.Name != tools[i].Name {
			t.Errorf("tool %d: expected name %q, got %q", i, tools[i].Name, openAITool.Function.Name)
		}
	}
}

func TestOpenAIToolSchemaTranslator_TranslateMultipleToolCalls(t *testing.T) {
	translator := NewOpenAIToolSchemaTranslator()

	toolCalls := []OpenAIToolCallResponse{
		{
			ID:   "call_1",
			Type: "function",
			Function: OpenAIToolCallFunction{
				Name:      "tool_a",
				Arguments: `{"key":"value1"}`,
			},
		},
		{
			ID:   "call_2",
			Type: "function",
			Function: OpenAIToolCallFunction{
				Name:      "tool_b",
				Arguments: `{"key":"value2"}`,
			},
		},
	}

	result, err := translator.TranslateMultipleToolCalls(toolCalls)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 content blocks, got %d", len(result))
	}

	if result[0].ToolUseID != "call_1" {
		t.Errorf("expected first ID 'call_1', got %s", result[0].ToolUseID)
	}
	if result[1].ToolName != "tool_b" {
		t.Errorf("expected second name 'tool_b', got %s", result[1].ToolName)
	}
}

func TestOpenAIToolSchemaTranslator_Integration(t *testing.T) {
	translator := NewOpenAIToolSchemaTranslator()

	// Define a realistic tool
	tool := ToolDefinition{
		Name:        "execute_query",
		Description: "Execute a database query",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"query": {
					"type": "string",
					"description": "SQL query to execute"
				},
				"database": {
					"type": "string",
					"enum": ["postgres", "mysql", "sqlite"]
				},
				"timeout": {
					"type": "integer",
					"minimum": 1,
					"maximum": 300,
					"default": 30
				}
			},
			"required": ["query", "database"]
		}`),
	}

	// Translate to OpenAI format
	openAIFormat, err := translator.TranslateToProvider(tool)
	if err != nil {
		t.Fatalf("failed to translate tool: %v", err)
	}

	// Verify it can be marshaled to JSON (as would be sent to API)
	jsonData, err := json.Marshal(openAIFormat)
	if err != nil {
		t.Fatalf("failed to marshal to JSON: %v", err)
	}

	// Verify structure is correct
	var parsed map[string]interface{}
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if parsed["type"] != "function" {
		t.Errorf("expected type 'function' in JSON")
	}

	funcMap := parsed["function"].(map[string]interface{})
	if funcMap["name"] != "execute_query" {
		t.Errorf("expected name 'execute_query' in function")
	}

	// Simulate receiving a tool call response
	responseToolCall := OpenAIToolCallResponse{
		ID:    "call_query_1",
		Type:  "function",
		Index: 0,
		Function: OpenAIToolCallFunction{
			Name:      "execute_query",
			Arguments: `{"query":"SELECT * FROM users","database":"postgres","timeout":60}`,
		},
	}

	block, err := translator.TranslateFromProvider(responseToolCall)
	if err != nil {
		t.Fatalf("failed to translate tool call: %v", err)
	}

	// Parse and verify the arguments
	parser := NewToolInputParser()
	query, err := parser.ParseString(block.ToolInput, "query")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}
	if query != "SELECT * FROM users" {
		t.Errorf("expected query 'SELECT * FROM users', got %s", query)
	}

	// Build a result
	resultBlock := ContentBlock{
		Type:         ContentTypeToolResult,
		ToolResultID: block.ToolUseID,
		ToolOutput:   `[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]`,
	}

	// Translate the result
	openAIResult, err := translator.TranslateToolResult(resultBlock)
	if err != nil {
		t.Fatalf("failed to translate result: %v", err)
	}

	msg := openAIResult.(OpenAIToolResultMessage)
	if msg.Role != "tool" {
		t.Errorf("expected role 'tool'")
	}
	if msg.ToolCallID != "call_query_1" {
		t.Errorf("expected tool_call_id 'call_query_1', got %s", msg.ToolCallID)
	}

	// Verify result can be marshaled
	resultJSON, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	var parsedResult map[string]interface{}
	if err := json.Unmarshal(resultJSON, &parsedResult); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}
	if parsedResult["role"] != "tool" {
		t.Errorf("expected role 'tool' in JSON")
	}
}

func TestJSONSchemaTypes(t *testing.T) {
	types := []struct {
		typ      JSONSchemaType
		expected string
	}{
		{JSONSchemaTypeString, "string"},
		{JSONSchemaTypeNumber, "number"},
		{JSONSchemaTypeInteger, "integer"},
		{JSONSchemaTypeBoolean, "boolean"},
		{JSONSchemaTypeArray, "array"},
		{JSONSchemaTypeObject, "object"},
		{JSONSchemaTypeNull, "null"},
	}

	for _, tc := range types {
		if string(tc.typ) != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, string(tc.typ))
		}
	}
}

func TestToolSchemaErrors(t *testing.T) {
	errors := []struct {
		err      error
		contains string
	}{
		{ErrInvalidToolDefinition, "invalid tool definition"},
		{ErrInvalidInputSchema, "invalid input schema"},
		{ErrUnsupportedSchemaType, "unsupported schema type"},
		{ErrMissingToolName, "missing tool name"},
		{ErrMissingToolID, "missing tool ID"},
		{ErrInvalidToolCall, "invalid tool call format"},
	}

	for _, tc := range errors {
		if tc.err.Error() != tc.contains {
			t.Errorf("expected error %q, got %q", tc.contains, tc.err.Error())
		}
	}
}

func TestBaseToolSchemaTranslator_Integration(t *testing.T) {
	translator := NewBaseToolSchemaTranslator("integration-test")

	// Define a tool
	tool := ToolDefinition{
		Name:        "search_files",
		Description: "Search for files matching a pattern",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pattern": {"type": "string", "description": "Glob pattern"},
				"recursive": {"type": "boolean", "default": true}
			},
			"required": ["pattern"]
		}`),
	}

	// Translate to provider format
	providerFormat, err := translator.TranslateToProvider(tool)
	if err != nil {
		t.Fatalf("failed to translate to provider: %v", err)
	}

	// Simulate a provider response (tool call)
	providerCall := map[string]interface{}{
		"id":   "call-integration",
		"name": "search_files",
		"arguments": map[string]interface{}{
			"pattern":   "*.go",
			"recursive": false,
		},
	}

	// Translate from provider format
	block, err := translator.TranslateFromProvider(providerCall)
	if err != nil {
		t.Fatalf("failed to translate from provider: %v", err)
	}

	// Verify the block
	if block.ToolName != "search_files" {
		t.Errorf("expected tool name 'search_files', got %s", block.ToolName)
	}

	// Parse the input
	parser := NewToolInputParser()
	pattern, err := parser.ParseString(block.ToolInput, "pattern")
	if err != nil {
		t.Fatalf("failed to parse pattern: %v", err)
	}
	if pattern != "*.go" {
		t.Errorf("expected pattern '*.go', got %s", pattern)
	}

	// Build a result
	resultMsg := NewToolResultBuilder(block.ToolUseID).
		WithJSONOutput([]string{"main.go", "tool_schema.go"}).
		BuildMessage()

	// Translate the result
	providerResult, err := translator.TranslateToolResult(resultMsg.Content[0])
	if err != nil {
		t.Fatalf("failed to translate result: %v", err)
	}

	resultMap := providerResult.(map[string]interface{})
	if resultMap["tool_use_id"] != "call-integration" {
		t.Errorf("expected tool_use_id 'call-integration', got %v", resultMap["tool_use_id"])
	}

	// Verify the provider format has expected fields
	providerMap := providerFormat.(map[string]interface{})
	if providerMap["name"] != "search_files" {
		t.Errorf("expected name 'search_files' in provider format")
	}
}
