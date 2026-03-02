// Package agent provides tool schema translation utilities.
// This file implements the base ToolSchemaTranslator functionality that converts
// between the unified OpenExec tool format and provider-specific formats.
package agent

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Common errors for tool schema translation
var (
	ErrInvalidToolDefinition = errors.New("invalid tool definition")
	ErrInvalidInputSchema    = errors.New("invalid input schema")
	ErrUnsupportedSchemaType = errors.New("unsupported schema type")
	ErrMissingToolName       = errors.New("missing tool name")
	ErrMissingToolID         = errors.New("missing tool ID")
	ErrInvalidToolCall       = errors.New("invalid tool call format")
)

// JSONSchemaType represents JSON Schema types
type JSONSchemaType string

const (
	JSONSchemaTypeString  JSONSchemaType = "string"
	JSONSchemaTypeNumber  JSONSchemaType = "number"
	JSONSchemaTypeInteger JSONSchemaType = "integer"
	JSONSchemaTypeBoolean JSONSchemaType = "boolean"
	JSONSchemaTypeArray   JSONSchemaType = "array"
	JSONSchemaTypeObject  JSONSchemaType = "object"
	JSONSchemaTypeNull    JSONSchemaType = "null"
)

// JSONSchema represents a JSON Schema definition for tool input parameters.
// This is a subset of JSON Schema Draft 7 commonly used by AI providers.
type JSONSchema struct {
	Type        JSONSchemaType         `json:"type,omitempty"`
	Description string                 `json:"description,omitempty"`
	Properties  map[string]*JSONSchema `json:"properties,omitempty"`
	Required    []string               `json:"required,omitempty"`
	Items       *JSONSchema            `json:"items,omitempty"`

	// String-specific
	Enum      []string `json:"enum,omitempty"`
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	Pattern   string   `json:"pattern,omitempty"`

	// Number/Integer-specific
	Minimum          *float64 `json:"minimum,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty"`
	ExclusiveMinimum *float64 `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum *float64 `json:"exclusiveMaximum,omitempty"`
	MultipleOf       *float64 `json:"multipleOf,omitempty"`

	// Array-specific
	MinItems    *int `json:"minItems,omitempty"`
	MaxItems    *int `json:"maxItems,omitempty"`
	UniqueItems bool `json:"uniqueItems,omitempty"`

	// Object-specific
	AdditionalProperties *bool `json:"additionalProperties,omitempty"`

	// Metadata
	Default interface{} `json:"default,omitempty"`
	Title   string      `json:"title,omitempty"`
	Ref     string      `json:"$ref,omitempty"`

	// Allow combining schemas (JSON Schema draft 7)
	AnyOf []*JSONSchema `json:"anyOf,omitempty"`
	OneOf []*JSONSchema `json:"oneOf,omitempty"`
	AllOf []*JSONSchema `json:"allOf,omitempty"`
}

// ParseJSONSchema parses a JSON Schema from raw JSON bytes.
func ParseJSONSchema(data json.RawMessage) (*JSONSchema, error) {
	if len(data) == 0 {
		return nil, ErrInvalidInputSchema
	}

	var schema JSONSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInputSchema, err)
	}

	return &schema, nil
}

// ToJSON converts the JSONSchema back to raw JSON bytes.
func (s *JSONSchema) ToJSON() (json.RawMessage, error) {
	return json.Marshal(s)
}

// Validate checks if the JSONSchema is valid for tool use.
func (s *JSONSchema) Validate() error {
	// Root schema for tools should typically be an object
	if s.Type != "" && s.Type != JSONSchemaTypeObject {
		// Allow if it's a reference or uses combining keywords
		if s.Ref == "" && len(s.AnyOf) == 0 && len(s.OneOf) == 0 && len(s.AllOf) == 0 {
			return fmt.Errorf("%w: root schema must be object type, got %s", ErrUnsupportedSchemaType, s.Type)
		}
	}

	// Validate nested properties
	for name, prop := range s.Properties {
		if prop == nil {
			return fmt.Errorf("%w: property %q is nil", ErrInvalidInputSchema, name)
		}
	}

	return nil
}

// IsRequired checks if a property is in the required list.
func (s *JSONSchema) IsRequired(property string) bool {
	for _, req := range s.Required {
		if req == property {
			return true
		}
	}
	return false
}

// GetPropertyNames returns all property names.
func (s *JSONSchema) GetPropertyNames() []string {
	names := make([]string, 0, len(s.Properties))
	for name := range s.Properties {
		names = append(names, name)
	}
	return names
}

// BaseToolSchemaTranslator provides common functionality for translating
// tool schemas between the unified format and provider-specific formats.
// Provider-specific translators should embed this struct and override
// methods as needed.
type BaseToolSchemaTranslator struct {
	// ProviderName identifies which provider this translator is for
	ProviderName string
}

// NewBaseToolSchemaTranslator creates a new BaseToolSchemaTranslator.
func NewBaseToolSchemaTranslator(providerName string) *BaseToolSchemaTranslator {
	return &BaseToolSchemaTranslator{
		ProviderName: providerName,
	}
}

// ValidateToolDefinition validates a ToolDefinition before translation.
func (t *BaseToolSchemaTranslator) ValidateToolDefinition(tool ToolDefinition) error {
	if tool.Name == "" {
		return ErrMissingToolName
	}

	// Validate the input schema if present
	if len(tool.InputSchema) > 0 {
		schema, err := ParseJSONSchema(tool.InputSchema)
		if err != nil {
			return fmt.Errorf("%w for tool %q: %v", ErrInvalidToolDefinition, tool.Name, err)
		}
		if err := schema.Validate(); err != nil {
			return fmt.Errorf("%w for tool %q: %v", ErrInvalidToolDefinition, tool.Name, err)
		}
	}

	return nil
}

// TranslateToProvider converts a unified ToolDefinition to a generic map format.
// Provider-specific translators should override this to produce their exact format.
func (t *BaseToolSchemaTranslator) TranslateToProvider(tool ToolDefinition) (interface{}, error) {
	if err := t.ValidateToolDefinition(tool); err != nil {
		return nil, err
	}

	// Return a generic representation that can be further transformed
	result := map[string]interface{}{
		"name":        tool.Name,
		"description": tool.Description,
	}

	if len(tool.InputSchema) > 0 {
		var schema interface{}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			return nil, fmt.Errorf("%w: failed to parse input schema: %v", ErrInvalidInputSchema, err)
		}
		result["parameters"] = schema
	}

	return result, nil
}

// TranslateFromProvider converts a provider tool call to unified format.
// This default implementation expects a map with standard fields.
// Provider-specific translators should override this for their format.
func (t *BaseToolSchemaTranslator) TranslateFromProvider(providerToolCall interface{}) (*ContentBlock, error) {
	callMap, ok := providerToolCall.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%w: expected map, got %T", ErrInvalidToolCall, providerToolCall)
	}

	// Extract common fields
	id, _ := callMap["id"].(string)
	name, _ := callMap["name"].(string)

	if name == "" {
		// Try alternative field names used by different providers
		name, _ = callMap["function"].(string)
	}

	if name == "" {
		return nil, fmt.Errorf("%w: missing tool name", ErrInvalidToolCall)
	}

	// Handle arguments/input
	var inputJSON json.RawMessage
	if args, ok := callMap["arguments"]; ok {
		switch v := args.(type) {
		case string:
			inputJSON = json.RawMessage(v)
		case map[string]interface{}:
			data, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to marshal arguments: %v", ErrInvalidToolCall, err)
			}
			inputJSON = data
		case json.RawMessage:
			inputJSON = v
		}
	} else if input, ok := callMap["input"]; ok {
		switch v := input.(type) {
		case string:
			inputJSON = json.RawMessage(v)
		case map[string]interface{}:
			data, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to marshal input: %v", ErrInvalidToolCall, err)
			}
			inputJSON = data
		case json.RawMessage:
			inputJSON = v
		}
	}

	return &ContentBlock{
		Type:      ContentTypeToolUse,
		ToolUseID: id,
		ToolName:  name,
		ToolInput: inputJSON,
	}, nil
}

// TranslateToolResult converts a unified tool result to a generic format.
// Provider-specific translators should override this for their format.
func (t *BaseToolSchemaTranslator) TranslateToolResult(result ContentBlock) (interface{}, error) {
	if result.Type != ContentTypeToolResult {
		return nil, fmt.Errorf("expected tool_result content type, got %s", result.Type)
	}

	if result.ToolResultID == "" {
		return nil, ErrMissingToolID
	}

	output := map[string]interface{}{
		"tool_use_id": result.ToolResultID,
		"content":     result.ToolOutput,
	}

	if result.ToolError != "" {
		output["is_error"] = true
		output["error"] = result.ToolError
	}

	return output, nil
}

// ToolCallExtractor provides utilities for extracting tool calls from responses.
type ToolCallExtractor struct{}

// NewToolCallExtractor creates a new ToolCallExtractor.
func NewToolCallExtractor() *ToolCallExtractor {
	return &ToolCallExtractor{}
}

// ExtractToolCalls extracts all tool calls from a response's content blocks.
func (e *ToolCallExtractor) ExtractToolCalls(content []ContentBlock) []ContentBlock {
	var calls []ContentBlock
	for _, block := range content {
		if block.Type == ContentTypeToolUse {
			calls = append(calls, block)
		}
	}
	return calls
}

// ExtractToolCallByID finds a specific tool call by its ID.
func (e *ToolCallExtractor) ExtractToolCallByID(content []ContentBlock, id string) *ContentBlock {
	for _, block := range content {
		if block.Type == ContentTypeToolUse && block.ToolUseID == id {
			return &block
		}
	}
	return nil
}

// ExtractToolCallsByName finds all tool calls with a specific name.
func (e *ToolCallExtractor) ExtractToolCallsByName(content []ContentBlock, name string) []ContentBlock {
	var calls []ContentBlock
	for _, block := range content {
		if block.Type == ContentTypeToolUse && block.ToolName == name {
			calls = append(calls, block)
		}
	}
	return calls
}

// ToolInputParser provides utilities for parsing tool input arguments.
type ToolInputParser struct{}

// NewToolInputParser creates a new ToolInputParser.
func NewToolInputParser() *ToolInputParser {
	return &ToolInputParser{}
}

// ParseAsMap parses tool input as a map[string]interface{}.
func (p *ToolInputParser) ParseAsMap(input json.RawMessage) (map[string]interface{}, error) {
	if len(input) == 0 {
		return make(map[string]interface{}), nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(input, &result); err != nil {
		return nil, fmt.Errorf("failed to parse tool input as map: %w", err)
	}
	return result, nil
}

// ParseString extracts a string value from tool input.
func (p *ToolInputParser) ParseString(input json.RawMessage, key string) (string, error) {
	m, err := p.ParseAsMap(input)
	if err != nil {
		return "", err
	}

	value, ok := m[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in tool input", key)
	}

	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("key %q is not a string, got %T", key, value)
	}

	return str, nil
}

// ParseInt extracts an integer value from tool input.
func (p *ToolInputParser) ParseInt(input json.RawMessage, key string) (int, error) {
	m, err := p.ParseAsMap(input)
	if err != nil {
		return 0, err
	}

	value, ok := m[key]
	if !ok {
		return 0, fmt.Errorf("key %q not found in tool input", key)
	}

	// JSON numbers are float64 by default
	switch v := value.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case int64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("key %q is not a number, got %T", key, value)
	}
}

// ParseBool extracts a boolean value from tool input.
func (p *ToolInputParser) ParseBool(input json.RawMessage, key string) (bool, error) {
	m, err := p.ParseAsMap(input)
	if err != nil {
		return false, err
	}

	value, ok := m[key]
	if !ok {
		return false, fmt.Errorf("key %q not found in tool input", key)
	}

	b, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("key %q is not a boolean, got %T", key, value)
	}

	return b, nil
}

// HasKey checks if a key exists in the tool input.
func (p *ToolInputParser) HasKey(input json.RawMessage, key string) bool {
	m, err := p.ParseAsMap(input)
	if err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}

// OpenAIToolSchemaTranslator provides OpenAI-specific tool schema translation.
// It translates tool definitions to OpenAI's function calling format and
// converts OpenAI tool call responses back to the unified format.
type OpenAIToolSchemaTranslator struct {
	*BaseToolSchemaTranslator
}

// NewOpenAIToolSchemaTranslator creates a new OpenAI tool schema translator.
func NewOpenAIToolSchemaTranslator() *OpenAIToolSchemaTranslator {
	return &OpenAIToolSchemaTranslator{
		BaseToolSchemaTranslator: NewBaseToolSchemaTranslator("openai"),
	}
}

// OpenAIFunctionTool represents a tool in OpenAI's function calling format.
type OpenAIFunctionTool struct {
	Type     string            `json:"type"`
	Function OpenAIFunctionDef `json:"function"`
}

// OpenAIFunctionDef represents a function definition in OpenAI's format.
type OpenAIFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

// OpenAIToolCallResponse represents a tool call from OpenAI's API response.
type OpenAIToolCallResponse struct {
	ID       string                 `json:"id"`
	Type     string                 `json:"type"`
	Index    int                    `json:"index,omitempty"`
	Function OpenAIToolCallFunction `json:"function"`
}

// OpenAIToolCallFunction represents the function details in a tool call.
type OpenAIToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAIToolResultMessage represents a tool result message in OpenAI's format.
type OpenAIToolResultMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
}

// TranslateToProvider converts a unified ToolDefinition to OpenAI's function format.
// OpenAI uses {"type": "function", "function": {...}} structure for tools.
func (t *OpenAIToolSchemaTranslator) TranslateToProvider(tool ToolDefinition) (interface{}, error) {
	if err := t.ValidateToolDefinition(tool); err != nil {
		return nil, err
	}

	// Build the OpenAI function tool structure
	openAITool := OpenAIFunctionTool{
		Type: "function",
		Function: OpenAIFunctionDef{
			Name:        tool.Name,
			Description: tool.Description,
		},
	}

	// Add parameters if present
	if len(tool.InputSchema) > 0 {
		openAITool.Function.Parameters = tool.InputSchema
	} else {
		// OpenAI expects an empty object schema if no parameters
		openAITool.Function.Parameters = json.RawMessage(`{"type":"object","properties":{}}`)
	}

	return openAITool, nil
}

// TranslateFromProvider converts an OpenAI tool call to the unified format.
// Accepts either an OpenAIToolCallResponse struct or a map[string]interface{}.
func (t *OpenAIToolSchemaTranslator) TranslateFromProvider(providerToolCall interface{}) (*ContentBlock, error) {
	// Handle different input types
	switch tc := providerToolCall.(type) {
	case OpenAIToolCallResponse:
		return t.translateOpenAIToolCall(tc)
	case *OpenAIToolCallResponse:
		if tc == nil {
			return nil, fmt.Errorf("%w: nil tool call", ErrInvalidToolCall)
		}
		return t.translateOpenAIToolCall(*tc)
	case map[string]interface{}:
		return t.translateFromMap(tc)
	default:
		return nil, fmt.Errorf("%w: expected OpenAIToolCallResponse or map, got %T", ErrInvalidToolCall, providerToolCall)
	}
}

// translateOpenAIToolCall converts an OpenAIToolCallResponse to a ContentBlock.
func (t *OpenAIToolSchemaTranslator) translateOpenAIToolCall(tc OpenAIToolCallResponse) (*ContentBlock, error) {
	if tc.Function.Name == "" {
		return nil, fmt.Errorf("%w: missing function name", ErrInvalidToolCall)
	}

	return &ContentBlock{
		Type:      ContentTypeToolUse,
		ToolUseID: tc.ID,
		ToolName:  tc.Function.Name,
		ToolInput: json.RawMessage(tc.Function.Arguments),
	}, nil
}

// translateFromMap converts a map representation of an OpenAI tool call.
func (t *OpenAIToolSchemaTranslator) translateFromMap(callMap map[string]interface{}) (*ContentBlock, error) {
	id, _ := callMap["id"].(string)

	// Extract function details - OpenAI nests under "function" key
	var funcName string
	var funcArgs string

	if funcObj, ok := callMap["function"].(map[string]interface{}); ok {
		funcName, _ = funcObj["name"].(string)
		if args, ok := funcObj["arguments"].(string); ok {
			funcArgs = args
		} else if argsMap, ok := funcObj["arguments"].(map[string]interface{}); ok {
			data, err := json.Marshal(argsMap)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to marshal arguments: %v", ErrInvalidToolCall, err)
			}
			funcArgs = string(data)
		}
	} else {
		// Fallback to direct name field (for compatibility)
		funcName, _ = callMap["name"].(string)
		if args, ok := callMap["arguments"].(string); ok {
			funcArgs = args
		} else if argsMap, ok := callMap["arguments"].(map[string]interface{}); ok {
			data, err := json.Marshal(argsMap)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to marshal arguments: %v", ErrInvalidToolCall, err)
			}
			funcArgs = string(data)
		}
	}

	if funcName == "" {
		return nil, fmt.Errorf("%w: missing function name", ErrInvalidToolCall)
	}

	return &ContentBlock{
		Type:      ContentTypeToolUse,
		ToolUseID: id,
		ToolName:  funcName,
		ToolInput: json.RawMessage(funcArgs),
	}, nil
}

// TranslateToolResult converts a unified tool result to OpenAI's format.
// OpenAI expects {"role": "tool", "content": "...", "tool_call_id": "..."}.
func (t *OpenAIToolSchemaTranslator) TranslateToolResult(result ContentBlock) (interface{}, error) {
	if result.Type != ContentTypeToolResult {
		return nil, fmt.Errorf("expected tool_result content type, got %s", result.Type)
	}

	if result.ToolResultID == "" {
		return nil, ErrMissingToolID
	}

	// Construct the content - include error prefix if there was an error
	content := result.ToolOutput
	if result.ToolError != "" {
		content = fmt.Sprintf("Error: %s", result.ToolError)
	}

	return OpenAIToolResultMessage{
		Role:       "tool",
		Content:    content,
		ToolCallID: result.ToolResultID,
	}, nil
}

// TranslateToolChoice converts a unified tool choice to OpenAI's format.
// OpenAI uses "auto", "none", "required", or {"type":"function","function":{"name":"..."}}
func (t *OpenAIToolSchemaTranslator) TranslateToolChoice(choice string) interface{} {
	switch choice {
	case "auto", "":
		return "auto"
	case "any":
		// "any" in the unified format means the model must use a tool
		return "required"
	case "none":
		return "none"
	default:
		// Specific tool name - format as structured choice
		return map[string]interface{}{
			"type": "function",
			"function": map[string]string{
				"name": choice,
			},
		}
	}
}

// TranslateMultipleTools converts multiple tool definitions to OpenAI's format.
func (t *OpenAIToolSchemaTranslator) TranslateMultipleTools(tools []ToolDefinition) ([]interface{}, error) {
	result := make([]interface{}, 0, len(tools))
	for _, tool := range tools {
		translated, err := t.TranslateToProvider(tool)
		if err != nil {
			return nil, fmt.Errorf("failed to translate tool %q: %w", tool.Name, err)
		}
		result = append(result, translated)
	}
	return result, nil
}

// TranslateMultipleToolCalls converts multiple OpenAI tool calls to unified format.
func (t *OpenAIToolSchemaTranslator) TranslateMultipleToolCalls(toolCalls []OpenAIToolCallResponse) ([]ContentBlock, error) {
	result := make([]ContentBlock, 0, len(toolCalls))
	for _, tc := range toolCalls {
		block, err := t.translateOpenAIToolCall(tc)
		if err != nil {
			return nil, err
		}
		result = append(result, *block)
	}
	return result, nil
}

// Compile-time verification that OpenAIToolSchemaTranslator implements ToolSchemaTranslator.
var _ ToolSchemaTranslator = (*OpenAIToolSchemaTranslator)(nil)

// ToolResultBuilder provides a fluent interface for building tool results.
type ToolResultBuilder struct {
	toolUseID string
	output    string
	err       error
	isError   bool
}

// NewToolResultBuilder creates a new ToolResultBuilder.
func NewToolResultBuilder(toolUseID string) *ToolResultBuilder {
	return &ToolResultBuilder{
		toolUseID: toolUseID,
	}
}

// WithOutput sets the output content.
func (b *ToolResultBuilder) WithOutput(output string) *ToolResultBuilder {
	b.output = output
	return b
}

// WithJSONOutput sets the output as JSON-encoded data.
func (b *ToolResultBuilder) WithJSONOutput(data interface{}) *ToolResultBuilder {
	bytes, err := json.Marshal(data)
	if err != nil {
		b.err = err
		b.isError = true
		return b
	}
	b.output = string(bytes)
	return b
}

// WithError sets an error result.
func (b *ToolResultBuilder) WithError(err error) *ToolResultBuilder {
	b.err = err
	b.isError = true
	return b
}

// Build creates the final ContentBlock.
func (b *ToolResultBuilder) Build() ContentBlock {
	block := ContentBlock{
		Type:         ContentTypeToolResult,
		ToolResultID: b.toolUseID,
		ToolOutput:   b.output,
	}

	if b.err != nil {
		block.ToolError = b.err.Error()
	}

	return block
}

// BuildMessage creates a Message containing the tool result.
func (b *ToolResultBuilder) BuildMessage() Message {
	return Message{
		Role:    RoleTool,
		Content: []ContentBlock{b.Build()},
	}
}
