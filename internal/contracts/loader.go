package contracts

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadFromYAML reads a blueprint YAML file and returns its
// feature_completeness_contracts section.
func LoadFromYAML(path string) (*ContractFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read blueprint yaml %q: %w", path, err)
	}
	return LoadFromBytes(data)
}

// LoadFromBytes parses a blueprint YAML document from raw bytes.
func LoadFromBytes(data []byte) (*ContractFile, error) {
	cf := &ContractFile{}
	if err := yaml.Unmarshal(data, cf); err != nil {
		return nil, fmt.Errorf("unmarshal contract yaml: %w", err)
	}
	if cf.Features == nil {
		cf.Features = map[string][]Operation{}
	}
	// Backfill the Feature field on each operation if missing so the
	// generator does not need to carry the feature name separately.
	for feature, ops := range cf.Features {
		for i := range ops {
			if ops[i].Feature == "" {
				ops[i].Feature = feature
			}
		}
		cf.Features[feature] = ops
	}
	return cf, nil
}

// IsEmpty returns true if the contract file has no features or no
// operations inside any feature.
func (c *ContractFile) IsEmpty() bool {
	if c == nil || len(c.Features) == 0 {
		return true
	}
	for _, ops := range c.Features {
		if len(ops) > 0 {
			return false
		}
	}
	return true
}
