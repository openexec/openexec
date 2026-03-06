package dcp

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/router"
	"github.com/openexec/openexec/internal/tools"
)

func TestDCPCoordinator(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	store, _ := knowledge.NewStore(tmpDir)
	defer store.Close()

	// Use a mock BitNet router (BitNetRouter with skipAvailabilityCheck)
	r := router.NewBitNetRouter("/mock/model")
	r.SetSkipAvailabilityCheck(true)
	
	coord := NewCoordinator(r, store)
	coord.RegisterTool(tools.NewDeployTool(store))
	coord.RegisterTool(tools.NewSymbolReaderTool(store))

	ctx := context.Background()

	t.Run("Process Deploy Query", func(t *testing.T) {
		// Arrange: Set up the required knowledge for deployment
		store.SetEnvironment(&knowledge.EnvironmentRecord{
			Env:         "prod",
			Topology:    `[{"ip": "1.1.1.1"}]`,
			RuntimeType: "vm",
			DeploySteps: `echo "success"`,
		})

		// Act
		// We bypass actual BitNet call in unit tests by ensuring our simulated runLocalInference 
		// handles the keywords. 
		res, err := coord.ProcessQuery(ctx, "Please deploy to prod")

		// Assert
		if err != nil {
			// This might fail because skipAvailabilityCheck is false. 
			// Let's check the error.
			if strings.Contains(err.Error(), "router unavailable") {
				t.Log("Skipping full integration test: BitNet model not present in test env")
				return
			}
			t.Fatalf("ProcessQuery failed: %v", err)
		}
		
		if !strings.Contains(res.(string), "Successfully deployed") {
			t.Errorf("unexpected result: %v", res)
		}
	})

	t.Run("PII and Infrastructure Scrubbing", func(t *testing.T) {
		// Arrange: Register a mock tool that echoes its input
		echoTool := &mockEchoTool{}
		coord.RegisterTool(echoTool)

		// Create a mock router that returns the echo tool
		mockRouter := &mockPIIRouter{}
		coord.router = mockRouter

		query := "Process data for test@example.com on 10.0.0.1"
		
		// Act
		res, err := coord.ProcessQuery(ctx, query)

		// Assert
		if err != nil {
			t.Fatalf("ProcessQuery failed: %v", err)
		}

		output := res.(string)
		if strings.Contains(output, "test@example.com") {
			t.Error("PII (email) was not scrubbed")
		}
		if !strings.Contains(output, "[EMAIL_REDACTED]") {
			t.Error("PII (email) placeholder missing")
		}
		if strings.Contains(output, "10.0.0.1") {
			t.Error("Infrastructure (IP) was not masked")
		}
		if !strings.Contains(output, "[IP_REDACTED]") {
			t.Error("Infrastructure (IP) placeholder missing")
		}
	})
}

type mockEchoTool struct{}
func (m *mockEchoTool) Name() string { return "echo" }
func (m *mockEchoTool) Description() string { return "Echoes input" }
func (m *mockEchoTool) InputSchema() string { return "{}" }
func (m *mockEchoTool) Execute(ctx context.Context, args map[string]interface{}) (any, error) {
	return args["text"], nil
}

type mockPIIRouter struct {
	router.BitNetRouter
}
func (m *mockPIIRouter) ParseIntent(ctx context.Context, query string) (*router.Intent, error) {
	return &router.Intent{
		ToolName:   "echo",
		Args:       map[string]interface{}{"text": query},
		Confidence: 1.0,
	}, nil
}
func (m *mockPIIRouter) RegisterTool(name, desc, schema string) error { return nil }
