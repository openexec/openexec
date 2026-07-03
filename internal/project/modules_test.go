package project

import "testing"

func boolPtr(b bool) *bool { return &b }

func TestModulesShouldLoad_DefaultEnabled(t *testing.T) {
	// Zero value (no modules block) → every module loads (backwards compatible).
	var m ModulesConfig
	for _, mod := range []string{"governance", "sre"} {
		if ok, reason := m.ShouldLoad(mod); !ok {
			t.Errorf("%s: expected enabled by default, got disabled: %s", mod, reason)
		}
	}
}

func TestModulesShouldLoad_ExplicitDisable(t *testing.T) {
	m := ModulesConfig{Governance: &ModuleToggle{Enabled: boolPtr(false)}}
	if ok, reason := m.ShouldLoad("governance"); ok || reason != "disabled by config" {
		t.Errorf("governance: expected disabled-by-config, got ok=%v reason=%q", ok, reason)
	}
	// A sibling module with no entry stays enabled.
	if ok, _ := m.ShouldLoad("sre"); !ok {
		t.Error("sre: expected enabled when only governance is disabled")
	}
}

func TestModulesShouldLoad_ExplicitEnable(t *testing.T) {
	m := ModulesConfig{SRE: &ModuleToggle{Enabled: boolPtr(true)}}
	if ok, _ := m.ShouldLoad("sre"); !ok {
		t.Error("sre: expected enabled when explicitly true")
	}
}

func TestModulesShouldLoad_EntitlementDeniesAfterEnabled(t *testing.T) {
	// The entitlement seam runs only after the enabled flag; disabled-by-config
	// short-circuits before it. Restore the default afterwards.
	orig := EntitlementCheck
	defer func() { EntitlementCheck = orig }()

	called := false
	EntitlementCheck = func(module, license string) error {
		called = true
		if module == "governance" && license == "expired" {
			return errModuleNotLicensed
		}
		return nil
	}

	m := ModulesConfig{Governance: &ModuleToggle{License: "expired"}}
	if ok, reason := m.ShouldLoad("governance"); ok || reason != errModuleNotLicensed.Error() {
		t.Errorf("governance: expected entitlement denial, got ok=%v reason=%q", ok, reason)
	}
	if !called {
		t.Error("EntitlementCheck was not consulted for an enabled module")
	}

	// A config-disabled module must NOT reach the entitlement check.
	called = false
	m2 := ModulesConfig{Governance: &ModuleToggle{Enabled: boolPtr(false), License: "expired"}}
	if ok, _ := m2.ShouldLoad("governance"); ok {
		t.Error("governance: expected disabled-by-config to win")
	}
	if called {
		t.Error("EntitlementCheck should not run when the module is disabled by config")
	}
}

// errModuleNotLicensed is a test-local sentinel for the entitlement seam.
var errModuleNotLicensed = errTest("module not licensed")

type errTest string

func (e errTest) Error() string { return string(e) }
