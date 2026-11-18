// Package channeldecl checks the repository's Console channel contract.
package channeldecl

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Validate checks that data declares a complete openexec-ci channel.
func Validate(data []byte) error {
	var declaration struct {
		Channels []struct {
			ID             string `yaml:"id"`
			Type           string `yaml:"type"`
			Repository     string `yaml:"repository"`
			Branch         string `yaml:"branch"`
			BearerTokenEnv string `yaml:"bearer_token_env"`
			Schedule       string `yaml:"schedule"`
			Timeout        string `yaml:"timeout"`
		} `yaml:"channels"`
	}
	if err := yaml.Unmarshal(data, &declaration); err != nil {
		return fmt.Errorf("parse channels: %w", err)
	}
	for _, channel := range declaration.Channels {
		if channel.ID != "openexec-ci" {
			continue
		}
		if channel.Type != "ci" {
			return fmt.Errorf("openexec-ci type must be ci")
		}
		for name, value := range map[string]string{
			"repository": channel.Repository, "branch": channel.Branch,
			"bearer_token_env": channel.BearerTokenEnv, "schedule": channel.Schedule, "timeout": channel.Timeout,
		} {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("openexec-ci missing %s", name)
			}
		}
		return nil
	}
	return fmt.Errorf("openexec-ci channel missing")
}
