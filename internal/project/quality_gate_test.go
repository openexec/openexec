package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/internal/execution/gates"
)

func TestInitializeInfersOnlyDeclaredChecks(t *testing.T) {
	cases := []struct {
		name    string
		files   map[string]string
		command string
	}{
		{"go-package", map[string]string{"go.mod": "module m\n", "cmd/x/main.go": "package main\n"}, "go vet ./..."},
		{"typescript-lint-script", map[string]string{"package.json": `{"scripts":{"lint":"eslint ."}}`, "tsconfig.json": "{}"}, "npm run lint"},
		// Near misses: nothing the project declares can be run as a check.
		{"go-only-vendor-testdata-hidden", map[string]string{"go.mod": "module m\n", "vendor/v/v.go": "package v\n", "testdata/t.go": "package t\n", ".cache/c.go": "package c\n", "_tmp/u.go": "package u\n"}, ""},
		{"go-mod-only", map[string]string{"go.mod": "module m\n"}, ""},
		{"blank-lint-script", map[string]string{"package.json": `{"scripts":{"lint":"  "}}`}, ""},
		{"lint-name-only-in-other-key", map[string]string{"package.json": `{"lint":"eslint .","scripts":{"lint:fix":"eslint --fix ."}}`}, ""},
		{"malformed-package-json", map[string]string{"package.json": `{"scripts":`}, ""},
		{"python", map[string]string{"pyproject.toml": "[project]\nname='x'\n", "x.py": "print(1)\n"}, ""},
		{"rust", map[string]string{"Cargo.toml": "[package]\nname='x'\n"}, ""},
		{"empty", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range tc.files {
				path := filepath.Join(dir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := Initialize("fixture", dir); err != nil {
				t.Fatal(err)
			}
			cfg, err := gates.LoadConfig(dir)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Project.Name != "fixture" {
				t.Fatalf("project name lost on reload: %+v", cfg.Project)
			}
			declared := cfg.GetEnabledGates()
			if tc.command == "" {
				if len(declared) != 0 || len(cfg.Quality.Custom) != 0 {
					t.Fatalf("declared %v / %+v with nothing to run", declared, cfg.Quality.Custom)
				}
				return
			}
			if len(declared) != 1 {
				t.Fatalf("declared %v", declared)
			}
			gate := cfg.GetGate(declared[0])
			if gate == nil || gate.Command != tc.command || gate.Mode != "blocking" {
				t.Fatalf("gate %+v, want %q", gate, tc.command)
			}
		})
	}
}
