package context

import (
	"context"
	"testing"
)

func TestNewGathererRegistry(t *testing.T) {
	registry := NewGathererRegistry()

	if registry == nil {
		t.Fatal("NewGathererRegistry() returned nil")
	}
	if registry.Count() != 0 {
		t.Errorf("Count() = %d, want 0 for new registry", registry.Count())
	}
}

func TestGathererRegistry_Register(t *testing.T) {
	registry := NewGathererRegistry()

	g := NewEnvironmentGatherer()
	registry.Register(g)

	if registry.Count() != 1 {
		t.Errorf("Count() = %d, want 1", registry.Count())
	}

	retrieved, ok := registry.Get(ContextTypeEnvironment)
	if !ok {
		t.Error("Get() should return registered gatherer")
	}
	if retrieved != g {
		t.Error("Get() should return the same gatherer instance")
	}
}

func TestGathererRegistry_Register_Replace(t *testing.T) {
	registry := NewGathererRegistry()

	g1 := NewEnvironmentGatherer()
	g2 := NewEnvironmentGatherer()

	registry.Register(g1)
	registry.Register(g2) // Should replace g1

	if registry.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (should replace)", registry.Count())
	}

	retrieved, _ := registry.Get(ContextTypeEnvironment)
	if retrieved != g2 {
		t.Error("Get() should return the replaced gatherer")
	}
}

func TestGathererRegistry_Get_NotFound(t *testing.T) {
	registry := NewGathererRegistry()

	_, ok := registry.Get(ContextTypeGitStatus)
	if ok {
		t.Error("Get() should return false for non-existent gatherer")
	}
}

func TestGathererRegistry_GetAll(t *testing.T) {
	registry := NewGathererRegistry()

	registry.Register(NewEnvironmentGatherer())
	registry.Register(NewGitStatusGatherer())
	registry.Register(NewDirectoryStructureGatherer())

	all := registry.GetAll()
	if len(all) != 3 {
		t.Errorf("GetAll() returned %d gatherers, want 3", len(all))
	}
}

func TestGathererRegistry_Types(t *testing.T) {
	registry := NewGathererRegistry()

	registry.Register(NewEnvironmentGatherer())
	registry.Register(NewGitStatusGatherer())

	types := registry.Types()
	if len(types) != 2 {
		t.Errorf("Types() returned %d types, want 2", len(types))
	}

	hasEnv := false
	hasGit := false
	for _, ct := range types {
		if ct == ContextTypeEnvironment {
			hasEnv = true
		}
		if ct == ContextTypeGitStatus {
			hasGit = true
		}
	}
	if !hasEnv || !hasGit {
		t.Error("Types() should include both registered types")
	}
}

func TestGathererRegistry_Remove(t *testing.T) {
	registry := NewGathererRegistry()

	registry.Register(NewEnvironmentGatherer())
	registry.Register(NewGitStatusGatherer())

	registry.Remove(ContextTypeEnvironment)

	if registry.Count() != 1 {
		t.Errorf("Count() = %d, want 1 after removal", registry.Count())
	}

	_, ok := registry.Get(ContextTypeEnvironment)
	if ok {
		t.Error("Get() should return false for removed gatherer")
	}
}

func TestGathererRegistry_Remove_NonExistent(t *testing.T) {
	registry := NewGathererRegistry()

	// Should not panic
	registry.Remove(ContextTypeGitStatus)

	if registry.Count() != 0 {
		t.Error("Remove of non-existent should not affect registry")
	}
}

func TestGathererRegistry_RunAll(t *testing.T) {
	tempDir := t.TempDir()

	registry := NewGathererRegistry()
	registry.Register(NewEnvironmentGatherer())

	results := registry.RunAll(context.Background(), tempDir)

	if len(results) != 1 {
		t.Errorf("RunAll() returned %d results, want 1", len(results))
	}

	if results[0].Error != nil {
		t.Errorf("RunAll() result error = %v", results[0].Error)
	}
}

func TestGathererRegistry_RunByType(t *testing.T) {
	tempDir := t.TempDir()

	registry := NewGathererRegistry()
	registry.Register(NewEnvironmentGatherer())
	registry.Register(NewDirectoryStructureGatherer())

	result, err := registry.RunByType(context.Background(), tempDir, ContextTypeEnvironment)
	if err != nil {
		t.Fatalf("RunByType() error = %v", err)
	}

	if result.Item.Type != ContextTypeEnvironment {
		t.Errorf("RunByType() result type = %v, want %v", result.Item.Type, ContextTypeEnvironment)
	}
}

func TestGathererRegistry_RunByType_NotFound(t *testing.T) {
	tempDir := t.TempDir()

	registry := NewGathererRegistry()

	_, err := registry.RunByType(context.Background(), tempDir, ContextTypeGitStatus)
	if err == nil {
		t.Error("RunByType() should return error for non-existent type")
	}
}

func TestDefaultRegistry(t *testing.T) {
	registry := DefaultRegistry()

	if registry == nil {
		t.Fatal("DefaultRegistry() returned nil")
	}

	// Should have multiple gatherers
	if registry.Count() < 5 {
		t.Errorf("DefaultRegistry() should have at least 5 gatherers, got %d", registry.Count())
	}

	// Check for specific gatherers
	expectedTypes := []ContextType{
		ContextTypeProjectInstructions,
		ContextTypeGitStatus,
		ContextTypeEnvironment,
		ContextTypeDirectoryStructure,
		ContextTypeRecentFiles,
		ContextTypePackageInfo,
	}

	for _, ct := range expectedTypes {
		if _, ok := registry.Get(ct); !ok {
			t.Errorf("DefaultRegistry() should include %s gatherer", ct)
		}
	}
}

func TestConfiguredRegistry(t *testing.T) {
	configs := []GathererConfig{
		{
			Type:      ContextTypeEnvironment,
			Name:      "Env",
			MaxTokens: 500,
			IsEnabled: true,
		},
		{
			Type:      ContextTypeGitStatus,
			Name:      "Git",
			MaxTokens: 1000,
			IsEnabled: true,
		},
		{
			Type:      ContextTypeGitDiff,
			Name:      "Diff",
			MaxTokens: 2000,
			IsEnabled: false, // Disabled
		},
	}

	registry := ConfiguredRegistry(configs)

	// Should have 2 gatherers (disabled one excluded)
	if registry.Count() != 2 {
		t.Errorf("ConfiguredRegistry() count = %d, want 2", registry.Count())
	}

	// Should not include disabled gatherer
	if _, ok := registry.Get(ContextTypeGitDiff); ok {
		t.Error("ConfiguredRegistry() should not include disabled gatherers")
	}
}

func TestConfiguredRegistry_EmptyConfigs(t *testing.T) {
	registry := ConfiguredRegistry(nil)

	if registry.Count() != 0 {
		t.Errorf("ConfiguredRegistry(nil) should have 0 gatherers, got %d", registry.Count())
	}
}

func TestGathererFactory_Create(t *testing.T) {
	factory := NewGathererFactory()

	tests := []struct {
		contextType ContextType
		wantErr     bool
	}{
		{ContextTypeProjectInstructions, false},
		{ContextTypeGitStatus, false},
		{ContextTypeGitDiff, false},
		{ContextTypeGitLog, false},
		{ContextTypeEnvironment, false},
		{ContextTypeDirectoryStructure, false},
		{ContextTypeRecentFiles, false},
		{ContextTypePackageInfo, false},
		{ContextType("unknown"), true},
		{ContextTypeSessionSummary, true}, // Not implemented yet
	}

	for _, tt := range tests {
		t.Run(string(tt.contextType), func(t *testing.T) {
			g, err := factory.Create(tt.contextType)
			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && g.Type() != tt.contextType {
				t.Errorf("Create() returned gatherer with type %v, want %v", g.Type(), tt.contextType)
			}
		})
	}
}

func TestGathererFactory_CreateAll(t *testing.T) {
	factory := NewGathererFactory()

	gatherers := factory.CreateAll()

	if len(gatherers) < 5 {
		t.Errorf("CreateAll() returned %d gatherers, want at least 5", len(gatherers))
	}

	// Check that all gatherers have valid types
	for _, g := range gatherers {
		if !g.Type().IsValid() {
			t.Errorf("CreateAll() returned gatherer with invalid type: %s", g.Type())
		}
	}
}

func TestGathererFactory_AvailableTypes(t *testing.T) {
	factory := NewGathererFactory()

	types := factory.AvailableTypes()

	if len(types) < 5 {
		t.Errorf("AvailableTypes() returned %d types, want at least 5", len(types))
	}

	// All types should be valid
	for _, ct := range types {
		if !ct.IsValid() {
			t.Errorf("AvailableTypes() returned invalid type: %s", ct)
		}
	}
}

func TestGathererRegistry_Concurrency(t *testing.T) {
	registry := NewGathererRegistry()

	// Test concurrent access
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			registry.Register(NewEnvironmentGatherer())
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			registry.Get(ContextTypeEnvironment)
			registry.Count()
			registry.GetAll()
		}
		done <- true
	}()

	// Wait for both
	<-done
	<-done

	// Should not panic
}
