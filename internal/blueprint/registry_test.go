package blueprint

import (
	"testing"
)

func TestRegistry_NewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if r.blueprints == nil {
		t.Fatal("blueprints map is nil")
	}
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	bp := &Blueprint{
		ID:           "test_bp",
		Name:         "Test Blueprint",
		InitialStage: "start",
		Stages: map[string]*Stage{
			"start": {Name: "start", OnSuccess: "complete"},
		},
	}

	err := r.Register(bp)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify it's registered
	got, ok := r.Get("test_bp")
	if !ok {
		t.Fatal("blueprint not found after registration")
	}
	if got.ID != "test_bp" {
		t.Errorf("got ID = %q, want %q", got.ID, "test_bp")
	}
}

func TestRegistry_Register_DuplicateID(t *testing.T) {
	r := NewRegistry()

	bp := &Blueprint{
		ID:           "test_bp",
		Name:         "Test Blueprint",
		InitialStage: "start",
		Stages: map[string]*Stage{
			"start": {Name: "start", OnSuccess: "complete"},
		},
	}

	err := r.Register(bp)
	if err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	// Try to register again with same ID
	bp2 := &Blueprint{
		ID:           "test_bp",
		Name:         "Another Blueprint",
		InitialStage: "begin",
		Stages: map[string]*Stage{
			"begin": {Name: "begin", OnSuccess: "complete"},
		},
	}

	err = r.Register(bp2)
	if err == nil {
		t.Fatal("expected error for duplicate ID, got nil")
	}
}

func TestRegistry_Register_InvalidBlueprint(t *testing.T) {
	r := NewRegistry()

	// Missing required fields
	bp := &Blueprint{
		ID: "invalid_bp",
		// Missing Name and InitialStage
	}

	err := r.Register(bp)
	if err == nil {
		t.Fatal("expected error for invalid blueprint, got nil")
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()

	bp, ok := r.Get("nonexistent")
	if ok {
		t.Error("expected ok = false for nonexistent blueprint")
	}
	if bp != nil {
		t.Error("expected nil blueprint for nonexistent ID")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()

	// Register multiple blueprints
	blueprints := []*Blueprint{
		{
			ID:           "zebra",
			Name:         "Zebra Blueprint",
			InitialStage: "start",
			Stages: map[string]*Stage{
				"start": {Name: "start", OnSuccess: "complete"},
			},
		},
		{
			ID:           "alpha",
			Name:         "Alpha Blueprint",
			InitialStage: "start",
			Stages: map[string]*Stage{
				"start": {Name: "start", OnSuccess: "complete"},
			},
		},
		{
			ID:           "middle",
			Name:         "Middle Blueprint",
			InitialStage: "start",
			Stages: map[string]*Stage{
				"start": {Name: "start", OnSuccess: "complete"},
			},
		},
	}

	for _, bp := range blueprints {
		if err := r.Register(bp); err != nil {
			t.Fatalf("Register failed: %v", err)
		}
	}

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("List returned %d blueprints, want 3", len(list))
	}

	// Verify sorted by ID
	expectedOrder := []string{"alpha", "middle", "zebra"}
	for i, bp := range list {
		if bp.ID != expectedOrder[i] {
			t.Errorf("list[%d].ID = %q, want %q", i, bp.ID, expectedOrder[i])
		}
	}
}

func TestRegistry_List_Empty(t *testing.T) {
	r := NewRegistry()

	list := r.List()
	if list == nil {
		t.Fatal("List returned nil, want empty slice")
	}
	if len(list) != 0 {
		t.Errorf("List returned %d blueprints, want 0", len(list))
	}
}

func TestRegistry_MustRegister_Success(t *testing.T) {
	r := NewRegistry()

	bp := &Blueprint{
		ID:           "test_bp",
		Name:         "Test Blueprint",
		InitialStage: "start",
		Stages: map[string]*Stage{
			"start": {Name: "start", OnSuccess: "complete"},
		},
	}

	// Should not panic
	r.MustRegister(bp)

	got, ok := r.Get("test_bp")
	if !ok {
		t.Fatal("blueprint not found after MustRegister")
	}
	if got.ID != "test_bp" {
		t.Errorf("got ID = %q, want %q", got.ID, "test_bp")
	}
}

func TestRegistry_MustRegister_Panics(t *testing.T) {
	r := NewRegistry()

	// Invalid blueprint
	bp := &Blueprint{
		ID: "invalid",
		// Missing required fields
	}

	defer func() {
		if recover() == nil {
			t.Fatal("MustRegister did not panic for invalid blueprint")
		}
	}()

	r.MustRegister(bp)
}

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	if r == nil {
		t.Fatal("DefaultRegistry returned nil")
	}

	// Verify standard_task is registered
	bp, ok := r.Get("standard_task")
	if !ok {
		t.Fatal("standard_task not found in default registry")
	}
	if bp.ID != "standard_task" {
		t.Errorf("got ID = %q, want %q", bp.ID, "standard_task")
	}

	// Verify quick_fix is registered
	bp, ok = r.Get("quick_fix")
	if !ok {
		t.Fatal("quick_fix not found in default registry")
	}
	if bp.ID != "quick_fix" {
		t.Errorf("got ID = %q, want %q", bp.ID, "quick_fix")
	}
}

func TestDefaultRegistry_Singleton(t *testing.T) {
	r1 := DefaultRegistry()
	r2 := DefaultRegistry()

	if r1 != r2 {
		t.Error("DefaultRegistry should return the same instance")
	}
}

func TestDefaultRegistry_ContainsValidBlueprints(t *testing.T) {
	r := DefaultRegistry()
	list := r.List()

	if len(list) < 2 {
		t.Fatalf("expected at least 2 blueprints, got %d", len(list))
	}

	// All blueprints should be valid
	for _, bp := range list {
		if err := bp.Validate(); err != nil {
			t.Errorf("blueprint %q is invalid: %v", bp.ID, err)
		}
	}
}

func TestRegistry_ConcurrentAccess(t *testing.T) {
	r := NewRegistry()

	// Register some initial blueprints
	for i := 0; i < 5; i++ {
		bp := &Blueprint{
			ID:           "bp" + string(rune('A'+i)),
			Name:         "Blueprint",
			InitialStage: "start",
			Stages: map[string]*Stage{
				"start": {Name: "start", OnSuccess: "complete"},
			},
		}
		r.MustRegister(bp)
	}

	done := make(chan bool)

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				r.Get("bpA")
				r.List()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
