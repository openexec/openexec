package infra

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeInfraYAML(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(ConfigFileName)), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const validYAML = `
enabled: true
environments:
  staging:
    risk_profile: low
    ansible:
      playbook_dir: /etc/openexec/playbooks
      inventory: /etc/openexec/inventory/staging.ini
      playbooks: [deploy_staging.yml]
    ssh:
      allowed_hosts: ["10.0.1.*"]
      queries:
        check_disk: [df, -h]
`

func TestLoad_MissingFileMeansDisabled(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil || cfg != nil {
		t.Errorf("missing file: want (nil, nil), got (%v, %v)", cfg, err)
	}
}

func TestLoad_DisabledMeansNil(t *testing.T) {
	dir := t.TempDir()
	writeInfraYAML(t, dir, "enabled: false\n")
	cfg, err := Load(dir)
	if err != nil || cfg != nil {
		t.Errorf("disabled: want (nil, nil), got (%v, %v)", cfg, err)
	}
}

func TestLoad_Valid(t *testing.T) {
	dir := t.TempDir()
	writeInfraYAML(t, dir, validYAML)
	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg == nil || !cfg.Enabled || cfg.Environments["staging"] == nil {
		t.Fatalf("config not loaded: %+v", cfg)
	}
	if cfg.Environments["staging"].SSH.Queries["check_disk"][0] != "df" {
		t.Error("ssh query argv not parsed")
	}
}

func TestLoad_UnknownKeyFailsLoud(t *testing.T) {
	dir := t.TempDir()
	// "playboks" is a typo: a security allowlist must fail loud on it, not
	// silently load an environment with no playbooks allowed (or all).
	writeInfraYAML(t, dir, strings.Replace(validYAML, "playbooks:", "playboks:", 1))
	if _, err := Load(dir); err == nil {
		t.Error("unknown yaml key must be a load error")
	}
}

func TestLoad_InvalidConfigFailsLoud(t *testing.T) {
	dir := t.TempDir()
	writeInfraYAML(t, dir, strings.Replace(validYAML, "risk_profile: low", "risk_profile: yolo", 1))
	if _, err := Load(dir); err == nil {
		t.Error("invalid risk_profile must be a load error")
	}
}

func TestLoadRegistry_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	writeInfraYAML(t, dir, validYAML)
	reg, err := LoadRegistry(dir)
	if err != nil || reg == nil {
		t.Fatalf("LoadRegistry: (%v, %v)", reg, err)
	}
	if _, err := reg.ResolveSSHQuery("staging", "10.0.1.7", "check_disk"); err != nil {
		t.Errorf("expected allowlisted query to resolve: %v", err)
	}

	none, err := LoadRegistry(t.TempDir())
	if err != nil || none != nil {
		t.Errorf("no config: want (nil, nil), got (%v, %v)", none, err)
	}
}

func TestExecRunner_ArgvIsLiteral(t *testing.T) {
	r := &ExecRunner{Timeout: 10 * time.Second}
	// A shell-metacharacter argument must arrive at the binary as one
	// literal string — this is the no-shell guarantee.
	out, code, err := r.Run(context.Background(), "echo", []string{"staging; rm -rf /"})
	if err != nil || code != 0 {
		t.Fatalf("echo failed: code=%d err=%v", code, err)
	}
	if strings.TrimSpace(out) != "staging; rm -rf /" {
		t.Errorf("argument was not passed literally: %q", out)
	}
}

func TestExecRunner_ExitCodeAndSpawnFailure(t *testing.T) {
	r := &ExecRunner{Timeout: 10 * time.Second}
	if _, code, err := r.Run(context.Background(), "false", nil); err == nil || code == 0 {
		t.Errorf("`false` should return non-zero: code=%d err=%v", code, err)
	}
	if _, code, err := r.Run(context.Background(), "openexec-no-such-binary-xyz", nil); err == nil || code != -1 {
		t.Errorf("missing binary should be a spawn failure: code=%d err=%v", code, err)
	}
}
