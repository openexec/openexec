package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config holds the global application configuration
type Config struct {
	LogLevel string
	DataDir  string
}

// InitializeConfig initializes viper configuration management
func InitializeConfig(cfgFile string) error {
	// Set configuration file path
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		// Default configuration location
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to determine home directory: %w", err)
		}

		configDir := filepath.Join(home, ".uaos")
		viper.AddConfigPath(configDir)
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")

		// Create config directory if it doesn't exist
		if err := os.MkdirAll(configDir, 0o700); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}
	}

	// Set defaults
	viper.SetDefault("log-level", "info")
	viper.SetDefault("data-dir", filepath.Join(os.Getenv("HOME"), ".uaos"))

	// Read configuration file (ignore if not found)
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Set environment variable prefix
	viper.SetEnvPrefix("UAOS")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()

	return nil
}

// GetConfig returns the current configuration
func GetConfig() *Config {
	return &Config{
		LogLevel: viper.GetString("log-level"),
		DataDir:  viper.GetString("data-dir"),
	}
}

// GetString retrieves a string value from config
func GetString(key string) string {
	return viper.GetString(key)
}

// GetBool retrieves a boolean value from config
func GetBool(key string) bool {
	return viper.GetBool(key)
}

// GetInt retrieves an integer value from config
func GetInt(key string) int {
	return viper.GetInt(key)
}

// Set sets a configuration value
func Set(key string, value interface{}) {
	viper.Set(key, value)
}
