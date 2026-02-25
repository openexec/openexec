// Package gates provides quality gate execution for task validation.
package gates

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the openexec.yaml configuration.
type Config struct {
	Project ProjectConfig `yaml:"project"`
	Quality QualityConfig `yaml:"quality"`
}

// ProjectConfig holds project metadata.
type ProjectConfig struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

// QualityConfig holds quality gate configuration.
type QualityConfig struct {
	Gates  []string     `yaml:"gates"`
	Custom []CustomGate `yaml:"custom"`
}

// CustomGate defines a custom quality gate.
type CustomGate struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
	Mode    string `yaml:"mode"` // "blocking" or "warning"
	Timeout int    `yaml:"timeout,omitempty"`
}

// LoadConfig loads the openexec.yaml configuration from the project directory.
func LoadConfig(projectDir string) (*Config, error) {
	// Try openexec.yaml first
	configPath := filepath.Join(projectDir, "openexec.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Try .openexec/openexec.yaml
		configPath = filepath.Join(projectDir, ".openexec", "openexec.yaml")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("openexec.yaml not found in %s", projectDir)
		}
	}

	data, err := os.ReadFile(configPath) // #nosec G304
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// GetGate returns a custom gate by name.
func (c *Config) GetGate(name string) *CustomGate {
	for i := range c.Quality.Custom {
		if c.Quality.Custom[i].Name == name {
			return &c.Quality.Custom[i]
		}
	}
	return nil
}

// GetEnabledGates returns the list of enabled gates.
func (c *Config) GetEnabledGates() []string {
	return c.Quality.Gates
}
