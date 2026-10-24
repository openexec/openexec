package project

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/quality"
)

// initGateName is the gate name a fresh openexec.yaml declares when init can
// infer a real check. The runner resolves names only through quality.custom,
// so the name is always written together with its command.
const initGateName = "lint"

// inferQualityGateCommand returns the check a fresh project can declare for
// its quality gate, or "" when none can be inferred safely. It reads only what
// the project itself declares: a Go module that has Go packages, or a
// package.json that defines a lint script. Anything else is left undeclared
// rather than guessed.
func inferQualityGateCommand(projectDir string) string {
	switch quality.DetectProjectType(projectDir) {
	case "go":
		// `go vet ./...` on a module with no packages prints a warning and
		// exits 0, which would verify nothing.
		if hasGoPackage(projectDir) {
			return "go vet ./..."
		}
	case "javascript", "typescript":
		if packageScript(projectDir, "lint") {
			return "npm run lint"
		}
	}
	return ""
}

// hasGoPackage reports whether ./... would match at least one package: a .go
// file outside the directories the go tool ignores.
func hasGoPackage(projectDir string) bool {
	found := errors.New("found")
	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != projectDir && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, ".go") && !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "_") {
			return found
		}
		return nil
	})
	return errors.Is(err, found)
}

// packageScript reports whether package.json defines a non-empty script.
func packageScript(projectDir, script string) bool {
	data, err := os.ReadFile(filepath.Join(projectDir, "package.json")) // #nosec G304
	if err != nil {
		return false
	}
	var manifest struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &manifest) != nil {
		return false
	}
	return strings.TrimSpace(manifest.Scripts[script]) != ""
}

// openexecYAML renders the openexec.yaml written by Initialize. With no
// inferred command it declares no gate, so the gate runner reports that
// nothing is configured to verify instead of failing on an undefined name.
func openexecYAML(projectName, command string) string {
	header := "project:\n  name: " + projectName + "\n  type: fullstack-webapp\n\n"
	if command == "" {
		return header + `quality:
  # openexec init found no check it could infer for this project, so no gate
  # is declared and quick_fix verification refuses until one is. Declare one
  # by listing its name under gates and defining it under custom, e.g.
  #   gates: [lint]
  #   custom:
  #     - name: lint
  #       command: <the command that checks this project>
  #       mode: blocking
  gates: []
`
	}
	return header + `quality:
  gates:
    - ` + initGateName + `
  custom:
    - name: ` + initGateName + `
      command: "` + command + `"
      mode: blocking
`
}
