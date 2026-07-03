// Package architecture enforces the OpenExec core/module dependency rule from
// docs/OPENEXEC_CORE_MODULE_STRATEGY.md. It reads package_boundaries.yaml,
// computes the actual import graph via `go list`, and fails on any boundary
// violation not recorded in the manifest's baseline (a ratchet: no NEW coupling,
// while the recorded debt is driven down by later migration steps).
package architecture

import (
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const modulePrefix = "github.com/openexec/openexec/"

type manifest struct {
	CompositionRoot []string            `yaml:"composition_root"`
	Modules         map[string][]string `yaml:"modules"`
	Undecided       []string            `yaml:"undecided"`
	Baseline        []string            `yaml:"baseline"`
}

// tier of a package: "root", "core", "module", or "undecided". For modules, name
// is the owning module.
func (m *manifest) classify(pkg string) (tier, name string) {
	best := ""
	match := func(prefix, t, n string) {
		if (pkg == prefix || strings.HasPrefix(pkg, prefix+"/")) && len(prefix) > len(best) {
			best, tier, name = prefix, t, n
		}
	}
	for _, p := range m.CompositionRoot {
		match(p, "root", "")
	}
	for mod, prefixes := range m.Modules {
		for _, p := range prefixes {
			match(p, "module", mod)
		}
	}
	for _, p := range m.Undecided {
		match(p, "undecided", "")
	}
	if tier == "" {
		return "core", ""
	}
	return tier, name
}

// violates reports whether importer P (tier tp/module mp) importing I (ti/mi) is
// a boundary violation.
func violates(tp, mp, ti, mi string) bool {
	if tp == "undecided" || ti == "undecided" || tp == "root" {
		return false // root may import anything; undecided is exempt either way
	}
	switch tp {
	case "core":
		return ti == "module" || ti == "root"
	case "module":
		if ti == "root" {
			return true
		}
		return ti == "module" && mi != mp
	}
	return false
}

func TestPackageBoundaries(t *testing.T) {
	data, err := os.ReadFile("package_boundaries.yaml")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var m manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	baseline := map[string]bool{}
	for _, b := range m.Baseline {
		baseline[strings.TrimSpace(b)] = true
	}

	out, err := exec.Command("go", "list", "-f", "{{.ImportPath}} {{join .Imports \" \"}}", modulePrefix+"...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}

	var violations []string
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pkg := strings.TrimPrefix(fields[0], modulePrefix)
		tp, mp := m.classify(pkg)
		for _, imp := range fields[1:] {
			if !strings.HasPrefix(imp, modulePrefix) {
				continue // stdlib / third-party
			}
			ip := strings.TrimPrefix(imp, modulePrefix)
			ti, mi := m.classify(ip)
			if violates(tp, mp, ti, mi) {
				edge := pkg + " -> " + ip
				if !seen[edge] {
					seen[edge] = true
					violations = append(violations, edge)
				}
			}
		}
	}
	sort.Strings(violations)

	// New violations = not in the baseline. These fail the build.
	var newV []string
	for _, v := range violations {
		if !baseline[v] {
			newV = append(newV, v)
		}
	}
	if len(newV) > 0 {
		t.Fatalf("NEW package-boundary violation(s) — core must not import modules, modules must not import each other.\n"+
			"Add them to package_boundaries.yaml `baseline:` ONLY as a deliberate, temporary debt entry:\n  %s",
			strings.Join(newV, "\n  "))
	}

	// Stale baseline entries = recorded debt that is no longer a violation. Not a
	// failure, but log so the ratchet can be tightened.
	for _, b := range m.Baseline {
		b = strings.TrimSpace(b)
		if b != "" && !seen[b] {
			t.Logf("baseline entry no longer a violation (remove it to tighten the ratchet): %s", b)
		}
	}
}
