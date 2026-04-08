// Package contracts provides a generator that emits runnable Vitest
// "feature completeness" contract tests from blueprint YAML metadata.
//
// The goal of the contract tests is to make the implement-stage prompt
// unable to ship stubs: every mutating operation is exercised against a
// real database, re-read out of it, tested for authorization, and
// hammered concurrently.
package contracts

// Operation mirrors the blueprint extractor.Operation type locally so
// the openexec binary does not need to import the blueprints module.
type Operation struct {
	ID            string   `yaml:"id" json:"id"`
	Feature       string   `yaml:"feature" json:"feature"`
	Method        string   `yaml:"method" json:"method"`
	Path          string   `yaml:"path" json:"path"`
	Mutates       []string `yaml:"mutates,omitempty" json:"mutates,omitempty"`
	ReadsFromDB   bool     `yaml:"reads_from_db,omitempty" json:"reads_from_db,omitempty"`
	MustPersist   bool     `yaml:"must_persist,omitempty" json:"must_persist,omitempty"`
	Authorization string   `yaml:"authorization,omitempty" json:"authorization,omitempty"`
	Triggers      []string `yaml:"triggers,omitempty" json:"triggers,omitempty"`
	Severity      string   `yaml:"severity,omitempty" json:"severity,omitempty"`
	Criticality   string   `yaml:"criticality,omitempty" json:"criticality,omitempty"`
	SourceRepos   []string `yaml:"source_repos,omitempty" json:"source_repos,omitempty"`
	Frequency     string   `yaml:"frequency,omitempty" json:"frequency,omitempty"`
}

// ContractFile is the subset of a blueprint YAML the generator cares about.
type ContractFile struct {
	Features map[string][]Operation `yaml:"feature_completeness_contracts"`
}

// IsMutating returns true if the operation mutates any DB state or
// uses a mutating HTTP verb.
func (o Operation) IsMutating() bool {
	if o.MustPersist || len(o.Mutates) > 0 {
		return true
	}
	switch o.Method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}
