package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPipelineConfig(t *testing.T) {
	dir := t.TempDir()
	pipelinesDir := filepath.Join(dir, "pipelines")
	os.MkdirAll(pipelinesDir, 0o755)

	yaml := `phases:
  - id: TD
    agent: clario
    workflow: technical-design
  - id: IM
    agent: spark
    workflow: implement
  - id: RV
    agent: blade
    workflow: review
    routes:
      spark: IM
      hon: RF
  - id: RF
    agent: hon
    workflow: refactor
  - id: FL
    agent: clario
    workflow: feedback-loop
`
	os.WriteFile(filepath.Join(pipelinesDir, "default.yaml"), []byte(yaml), 0o644)

	def, err := LoadPipelineConfig(os.DirFS(dir), "default")
	if err != nil {
		t.Fatalf("LoadPipelineConfig: %v", err)
	}

	if len(def.Phases) != 5 {
		t.Fatalf("phases count = %d, want 5", len(def.Phases))
	}

	// Check order.
	ids := make([]string, len(def.Phases))
	for i, p := range def.Phases {
		ids[i] = p.ID
	}
	expected := []string{"TD", "IM", "RV", "RF", "FL"}
	for i, want := range expected {
		if ids[i] != want {
			t.Errorf("phase[%d] = %s, want %s", i, ids[i], want)
		}
	}

	// Check RV routes.
	rv := def.Phases[2]
	if rv.Routes["spark"] != "IM" {
		t.Errorf("RV route spark = %s, want IM", rv.Routes["spark"])
	}
	if rv.Routes["hon"] != "RF" {
		t.Errorf("RV route hon = %s, want RF", rv.Routes["hon"])
	}
}

func TestLoadPipelineConfigWithMaxIterations(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "pipelines"), 0o755)

	yaml := `phases:
  - id: A
    agent: a
    workflow: wf-a
    max_iterations: 20
  - id: B
    agent: b
    workflow: wf-b
`
	os.WriteFile(filepath.Join(dir, "pipelines", "custom.yaml"), []byte(yaml), 0o644)

	def, err := LoadPipelineConfig(os.DirFS(dir), "custom")
	if err != nil {
		t.Fatalf("LoadPipelineConfig: %v", err)
	}

	if def.Phases[0].MaxIterations != 20 {
		t.Errorf("phase A max_iterations = %d, want 20", def.Phases[0].MaxIterations)
	}
	if def.Phases[1].MaxIterations != 0 {
		t.Errorf("phase B max_iterations = %d, want 0", def.Phases[1].MaxIterations)
	}
}

func TestLoadPipelineConfigNotFound(t *testing.T) {
	_, err := LoadPipelineConfig(os.DirFS(t.TempDir()), "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing pipeline config")
	}
}

func TestLoadPipelineConfigInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "pipelines"), 0o755)
	os.WriteFile(filepath.Join(dir, "pipelines", "bad.yaml"), []byte(":::not yaml"), 0o644)

	_, err := LoadPipelineConfig(os.DirFS(dir), "bad")
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestValidateEmpty(t *testing.T) {
	def := &PipelineDef{}
	err := def.Validate()
	if err == nil {
		t.Fatal("expected error for empty pipeline")
	}
}

func TestValidateDuplicatePhaseID(t *testing.T) {
	def := &PipelineDef{
		Phases: []PhaseDef{
			{ID: "A", Agent: "a", Workflow: "wf"},
			{ID: "A", Agent: "b", Workflow: "wf"},
		},
	}
	err := def.Validate()
	if err == nil {
		t.Fatal("expected error for duplicate phase ID")
	}
}

func TestValidateMissingID(t *testing.T) {
	def := &PipelineDef{
		Phases: []PhaseDef{
			{Agent: "a", Workflow: "wf"},
		},
	}
	err := def.Validate()
	if err == nil {
		t.Fatal("expected error for missing phase ID")
	}
}

func TestValidateMissingAgent(t *testing.T) {
	def := &PipelineDef{
		Phases: []PhaseDef{
			{ID: "A", Workflow: "wf"},
		},
	}
	err := def.Validate()
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
}

func TestValidateMissingWorkflow(t *testing.T) {
	def := &PipelineDef{
		Phases: []PhaseDef{
			{ID: "A", Agent: "a"},
		},
	}
	err := def.Validate()
	if err == nil {
		t.Fatal("expected error for missing workflow")
	}
}

func TestValidateInvalidRouteTarget(t *testing.T) {
	def := &PipelineDef{
		Phases: []PhaseDef{
			{ID: "A", Agent: "a", Workflow: "wf", Routes: map[string]string{"next": "NONEXISTENT"}},
		},
	}
	err := def.Validate()
	if err == nil {
		t.Fatal("expected error for invalid route target")
	}
}

func TestValidateValidRouteTarget(t *testing.T) {
	def := &PipelineDef{
		Phases: []PhaseDef{
			{ID: "A", Agent: "a", Workflow: "wf", Routes: map[string]string{"next": "B"}},
			{ID: "B", Agent: "b", Workflow: "wf"},
		},
	}
	err := def.Validate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultPipelineDefValid(t *testing.T) {
	def := DefaultPipelineDef()
	if err := def.Validate(); err != nil {
		t.Fatalf("DefaultPipelineDef validation: %v", err)
	}
}
