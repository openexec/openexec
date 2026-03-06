package loop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestLoop_QualityGates(t *testing.T) {
	mockPath, _ := filepath.Abs("testdata/mock_claude")
	tmpDir := t.TempDir()

	// 1. Success case
	t.Run("Gates Pass", func(t *testing.T) {
		// Create openexec.yaml that passes
		yamlContent := `
project:
  name: "test"
quality:
  gates: ["pass"]
  custom:
    - name: "pass"
      command: "exit 0"
`
		os.WriteFile(filepath.Join(tmpDir, "openexec.yaml"), []byte(yamlContent), 0644)

		cfg := DefaultConfig()
		cfg.WorkDir = tmpDir
		cfg.CommandName = mockPath
		cfg.CommandArgs = []string{"signal-complete"}
		cfg.QualityGates = true
		
		l, events := New(cfg)
		
		done := make(chan struct{})
		var foundPassed bool
		var foundComplete bool
		go func() {
			for e := range events {
				if e.Type == EventGatesPassed {
					foundPassed = true
				}
				if e.Type == EventComplete {
					foundComplete = true
				}
			}
			close(done)
		}()
		
		err := l.Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		<-done
		
		if !foundPassed {
			t.Error("expected EventGatesPassed")
		}
		if !foundComplete {
			t.Error("expected EventComplete")
		}
	})

	// 2. Retry then pass
	t.Run("Gates Fail then Pass", func(t *testing.T) {
		// This is tricky because we need to change the file between iterations
		// But Loop doesn't give us an easy hook. 
		// We can use a script that checks for a file existence.
		
		passFile := filepath.Join(tmpDir, "pass_now")
		os.Remove(passFile)

		yamlContent := fmt.Sprintf(`
project:
  name: "test"
quality:
  gates: ["retry"]
  custom:
    - name: "retry"
      command: "if [ -f %s ]; then exit 0; else touch %s; exit 1; fi"
`, passFile, passFile)
		os.WriteFile(filepath.Join(tmpDir, "openexec.yaml"), []byte(yamlContent), 0644)

		cfg := DefaultConfig()
		cfg.WorkDir = tmpDir
		cfg.CommandName = mockPath
		cfg.CommandArgs = []string{"signal-complete"}
		cfg.QualityGates = true
		cfg.MaxGateRetries = 2
		
		l, events := New(cfg)
		
		done := make(chan struct{})
		var fixAttempts int
		var foundComplete bool
		go func() {
			for e := range events {
				if e.Type == EventGatesFixing {
					fixAttempts++
				}
				if e.Type == EventComplete {
					foundComplete = true
				}
			}
			close(done)
		}()
		
		err := l.Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		<-done
		
		if fixAttempts != 1 {
			t.Errorf("expected 1 fix attempt, got %d", fixAttempts)
		}
		if !foundComplete {
			t.Error("expected EventComplete eventually")
		}
	})
}
