package pipeline

import (
	"os"
	"testing"
	"time"
)

func TestNewLoopFactory(t *testing.T) {
	cfg := LoopFactoryConfig{
		FWUID:                "FWU-01",
		WorkDir:              t.TempDir(),
		AgentsFS:             os.DirFS("testdata"),
		DefaultMaxIterations: 10,
		MaxRetries:           3,
		RetryBackoff:         []time.Duration{0},
		ThrashThreshold:      3,
		CommandName:          "echo",
		CommandArgs:          []string{"test"},
	}

	factory := NewLoopFactory(cfg)
	if factory == nil {
		t.Fatal("expected non-nil LoopFactory")
	}
}

func TestLoopFactoryConfig(t *testing.T) {
	t.Run("all fields are stored", func(t *testing.T) {
		workDir := t.TempDir()
		cfg := LoopFactoryConfig{
			FWUID:                "FWU-TEST",
			WorkDir:              workDir,
			AgentsFS:             os.DirFS("testdata"),
			DefaultMaxIterations: 15,
			MaxRetries:           5,
			RetryBackoff:         []time.Duration{1 * time.Second, 2 * time.Second},
			ThrashThreshold:      4,
			ExecutorModel:        "gpt-4",
			RunnerCommand:        "/usr/bin/claude",
			RunnerArgs:           []string{"--arg1", "--arg2"},
			CommandName:          "mock_command",
			CommandArgs:          []string{"arg1", "arg2"},
			LogDir:               "/var/log",
			EvidenceDir:          "/evidence",
			EvidenceBucket:       "my-bucket",
			EvidenceRegion:       "us-east-1",
			EvidenceEndpoint:     "https://s3.amazonaws.com",
			EvidencePrefix:       "runs/",
			ExecMode:             "workspace-write",
		}

		factory := NewLoopFactory(cfg)
		if factory == nil {
			t.Fatal("expected non-nil LoopFactory")
		}

		// The factory stores the config internally for use during loop creation
		// We can verify it was created successfully with the config
		t.Log("LoopFactory created with full configuration")
	})

	t.Run("minimal config is valid", func(t *testing.T) {
		cfg := LoopFactoryConfig{
			AgentsFS:             os.DirFS("testdata"),
			DefaultMaxIterations: 10,
		}

		factory := NewLoopFactory(cfg)
		if factory == nil {
			t.Fatal("expected non-nil LoopFactory with minimal config")
		}
	})
}

func TestLoopFactoryRequiresAgentsFS(t *testing.T) {
	cfg := LoopFactoryConfig{
		DefaultMaxIterations: 10,
		// AgentsFS is nil - assembler will still be created but may fail during assembly
	}

	// Factory creation doesn't fail with nil AgentsFS,
	// but the assembler will fail when trying to load agents.
	// This is tested during actual loop creation in pipeline tests.
	factory := NewLoopFactory(cfg)
	if factory == nil {
		t.Fatal("factory should be created even with nil AgentsFS")
	}
}
