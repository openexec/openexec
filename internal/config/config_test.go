package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestInitializeConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.yaml")

	content := `
log-level: debug
data-dir: /tmp/data
`
	err := os.WriteFile(cfgFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Reset viper for testing
	viper.Reset()

	err = InitializeConfig(cfgFile)
	if err != nil {
		t.Fatalf("InitializeConfig failed: %v", err)
	}

	cfg := GetConfig()
	if cfg.LogLevel != "debug" {
		t.Errorf("got log level %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.DataDir != "/tmp/data" {
		t.Errorf("got data dir %q, want %q", cfg.DataDir, "/tmp/data")
	}
}

func TestConfigDefaults(t *testing.T) {
	viper.Reset()

	// Use an empty file for defaults
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "empty_config.yaml")
	err := os.WriteFile(cfgFile, []byte(""), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = InitializeConfig(cfgFile)
	if err != nil {
		t.Fatalf("InitializeConfig failed: %v", err)
	}

	cfg := GetConfig()
	if cfg.LogLevel != "info" {
		t.Errorf("got default log level %q, want %q", cfg.LogLevel, "info")
	}

	// DataDir default is $HOME/.uaos
	home := os.Getenv("HOME")
	expectedDataDir := filepath.Join(home, ".uaos")
	if cfg.DataDir != expectedDataDir {
		t.Errorf("got default data dir %q, want %q", cfg.DataDir, expectedDataDir)
	}
}

func TestConfigEnvOverrides(t *testing.T) {
	viper.Reset()

	os.Setenv("UAOS_LOG_LEVEL", "error")
	defer os.Unsetenv("UAOS_LOG_LEVEL")

	// Create an empty file to ensure it reads something
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(cfgFile, []byte(""), 0644)

	err := InitializeConfig(cfgFile)
	if err != nil {
		t.Fatalf("InitializeConfig failed: %v", err)
	}

	if GetString("log-level") != "error" {
		t.Errorf("got log level %q, want %q", GetString("log-level"), "error")
	}
}

func TestGettersAndSetters(t *testing.T) {
	viper.Reset()

	Set("test-key", "test-value")
	if GetString("test-key") != "test-value" {
		t.Errorf("GetString failed: got %q", GetString("test-key"))
	}

	Set("test-bool", true)
	if !GetBool("test-bool") {
		t.Error("GetBool failed")
	}

	Set("test-int", 42)
	if GetInt("test-int") != 42 {
		t.Errorf("GetInt failed: got %d", GetInt("test-int"))
	}
}

func TestInitializeConfig_DefaultPath(t *testing.T) {
	viper.Reset()

	tmpHome := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", oldHome)

	// Call with empty string to trigger default path logic
	err := InitializeConfig("")
	if err != nil {
		t.Fatalf("InitializeConfig failed: %v", err)
	}

	configDir := filepath.Join(tmpHome, ".uaos")
	if _, err := os.Stat(configDir); err != nil {
		t.Errorf("default config directory not created: %v", err)
	}
}

func TestInitializeConfig_FileReadError(t *testing.T) {
	viper.Reset()
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "unreadable.yaml")

	// Create a directory with the same name to cause a read error
	os.Mkdir(cfgFile, 0755)

	err := InitializeConfig(cfgFile)
	if err == nil {
		t.Error("expected error reading unreadable file (is a directory)")
	}
}
