package workflow

import (
	"strings"
	"testing"
)

func TestValidateAllProvided(t *testing.T) {
	declared := map[string]string{"intent": "desc", "stopping_criteria": "desc"}
	provided := map[string]string{"intent": "value1", "stopping_criteria": "value2"}
	if err := Validate(declared, provided); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestValidateMissingParam(t *testing.T) {
	declared := map[string]string{"intent": "desc", "stopping_criteria": "desc"}
	provided := map[string]string{"intent": "value1"}
	err := Validate(declared, provided)
	if err == nil {
		t.Error("expected error for missing param, got nil")
	}
	if !strings.Contains(err.Error(), "stopping_criteria") {
		t.Errorf("error should mention missing param, got: %v", err)
	}
}

func TestValidateNilDeclared(t *testing.T) {
	if err := Validate(nil, map[string]string{"extra": "val"}); err != nil {
		t.Errorf("Validate with nil declared: %v", err)
	}
}

func TestExpandAllParams(t *testing.T) {
	text := "Intent: {{intent}}\nStop when: {{stopping_criteria}}"
	params := map[string]string{"intent": "validate tests", "stopping_criteria": "score meets standards"}

	result, err := Expand(text, params)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}

	if !strings.Contains(result, "validate tests") {
		t.Errorf("result missing expanded intent: %q", result)
	}
	if !strings.Contains(result, "score meets standards") {
		t.Errorf("result missing expanded stopping_criteria: %q", result)
	}
	if strings.Contains(result, "{{") {
		t.Errorf("result still contains placeholders: %q", result)
	}
}

func TestExpandUnreplacedPlaceholder(t *testing.T) {
	text := "Intent: {{intent}}\nExtra: {{unknown}}"
	params := map[string]string{"intent": "value"}

	_, err := Expand(text, params)
	if err == nil {
		t.Error("expected error for unreplaced placeholder, got nil")
	}
	if !strings.Contains(err.Error(), "{{unknown}}") {
		t.Errorf("error should mention unreplaced placeholder, got: %v", err)
	}
}

func TestExpandNoParamsNoPlaceholders(t *testing.T) {
	text := "Plain text without placeholders"
	result, err := Expand(text, nil)
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if result != text {
		t.Errorf("result = %q, want %q", result, text)
	}
}

func TestExpandNoParamsWithPlaceholders(t *testing.T) {
	text := "Has {{placeholder}}"
	_, err := Expand(text, nil)
	if err == nil {
		t.Error("expected error for placeholder with no params, got nil")
	}
}
