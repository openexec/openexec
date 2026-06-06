package infra

import (
	"reflect"
	"strings"
	"testing"
)

// testConfig mirrors the roadmap's example: staging is permissive, prod is
// narrow. Used by every resolution test.
func testConfig() *Config {
	return &Config{
		Enabled: true,
		Environments: map[string]*EnvConfig{
			"staging": {
				RiskProfile: "low",
				Ansible: &AnsibleConfig{
					PlaybookDir: "/etc/openexec/playbooks",
					Inventory:   "/etc/openexec/inventory/staging.ini",
					Playbooks:   []string{"deploy_staging.yml", "rolling_restart.yml"},
				},
				Salt: &SaltConfig{
					Targets: []string{"web*", "db1.staging"},
					States:  []string{"nginx.setup", "app.deploy"},
				},
				SSH: &SSHConfig{
					AllowedHosts: []string{"10.0.1.*", "*.staging.internal"},
					Queries: map[string][]string{
						"check_disk":    {"df", "-h"},
						"check_service": {"systemctl", "status", "nginx", "--no-pager"},
					},
				},
				Terraform: &TerraformConfig{
					WorkingDir:       "./infra/staging",
					AllowedVariables: []string{"instance_count", "node_type"},
				},
			},
			"production": {
				RiskProfile: "high",
				Ansible: &AnsibleConfig{
					PlaybookDir: "/etc/openexec/playbooks",
					Inventory:   "/etc/openexec/inventory/prod.ini",
					Playbooks:   []string{"rolling_restart.yml"},
				},
			},
		},
	}
}

func mustRegistry(t *testing.T) *Registry {
	t.Helper()
	reg, err := NewRegistry(testConfig())
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return reg
}

func TestResolveAnsiblePlaybook_ArgvExact(t *testing.T) {
	reg := mustRegistry(t)
	cmd, err := reg.ResolveAnsiblePlaybook("staging", "deploy_staging.yml", "web*", false)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	wantArgs := []string{
		"-i", "/etc/openexec/inventory/staging.ini",
		"/etc/openexec/playbooks/deploy_staging.yml",
		"--limit", "web*",
	}
	if cmd.Binary != "ansible-playbook" || !reflect.DeepEqual(cmd.Args, wantArgs) {
		t.Errorf("argv mismatch:\n got %q %q\nwant ansible-playbook %q", cmd.Binary, cmd.Args, wantArgs)
	}
	if !cmd.ApplyClass {
		t.Error("a non-check playbook run must be apply-class")
	}
	if cmd.RiskProfile != "low" {
		t.Errorf("risk profile = %q, want low", cmd.RiskProfile)
	}
}

func TestResolveAnsiblePlaybook_CheckIsNotApplyClass(t *testing.T) {
	reg := mustRegistry(t)
	cmd, err := reg.ResolveAnsiblePlaybook("staging", "deploy_staging.yml", "", true)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cmd.ApplyClass {
		t.Error("--check dry-run must not be apply-class")
	}
	if cmd.Args[len(cmd.Args)-1] != "--check" {
		t.Errorf("expected trailing --check, got %q", cmd.Args)
	}
}

func TestResolveAnsiblePlaybook_DenyByDefault(t *testing.T) {
	reg := mustRegistry(t)
	hostile := []struct {
		env, playbook, limit string
		desc                 string
	}{
		{"staging", "../../etc/passwd", "", "path traversal playbook"},
		{"staging", "wipe_all.yml", "", "playbook not in enum"},
		{"production", "deploy_staging.yml", "", "playbook allowed in staging but not production"},
		{"prod", "rolling_restart.yml", "", "unknown environment"},
		{"staging", "deploy_staging.yml", "web; rm -rf /", "shell metacharacters in limit"},
		{"staging", "deploy_staging.yml", "$(reboot)", "command substitution in limit"},
	}
	for _, h := range hostile {
		if _, err := reg.ResolveAnsiblePlaybook(h.env, h.playbook, h.limit, false); err == nil {
			t.Errorf("%s: expected rejection, got nil error", h.desc)
		}
	}
}

func TestResolveSaltState_ArgvAndDenials(t *testing.T) {
	reg := mustRegistry(t)
	cmd, err := reg.ResolveSaltState("staging", "app.deploy", "web*", false)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := []string{"web*", "state.apply", "app.deploy"}
	if cmd.Binary != "salt" || !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("argv mismatch: got %q %q want salt %q", cmd.Binary, cmd.Args, want)
	}
	if !cmd.ApplyClass {
		t.Error("state.apply without test=True must be apply-class")
	}

	// test=True is a dry-run
	dry, err := reg.ResolveSaltState("staging", "app.deploy", "web*", true)
	if err != nil {
		t.Fatalf("resolve test: %v", err)
	}
	if dry.ApplyClass || dry.Args[len(dry.Args)-1] != "test=True" {
		t.Errorf("test=True run must be dry-run with trailing test=True, got applyClass=%v args=%q", dry.ApplyClass, dry.Args)
	}

	// Denials: caller-invented target, state not in enum, destructive verb.
	if _, err := reg.ResolveSaltState("staging", "app.deploy", "*", false); err == nil {
		t.Error("salt target '*' is not in the allowlist and must be rejected")
	}
	if _, err := reg.ResolveSaltState("staging", "cmd.run", "web*", false); err == nil {
		t.Error("state cmd.run is not in the allowlist and must be rejected")
	}
}

func TestResolveSSHQuery_ArgvAndDenials(t *testing.T) {
	reg := mustRegistry(t)
	cmd, err := reg.ResolveSSHQuery("staging", "10.0.1.42", "check_disk")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := []string{"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "10.0.1.42", "df", "-h"}
	if cmd.Binary != "ssh" || !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("argv mismatch: got %q %q want ssh %q", cmd.Binary, cmd.Args, want)
	}
	if cmd.ApplyClass {
		t.Error("ssh queries are read-class by contract")
	}

	hostile := []struct{ host, query, desc string }{
		{"-oProxyCommand=evil", "check_disk", "ssh option injection via host"},
		{"10.0.2.42", "check_disk", "host outside allowed glob"},
		{"10.0.1.42", "drop_database", "query not in allowlist"},
		{"10.0.1.42; reboot", "check_disk", "shell metacharacters in host"},
		{"host name", "check_disk", "whitespace in host"},
	}
	for _, h := range hostile {
		if _, err := reg.ResolveSSHQuery("staging", h.host, h.query); err == nil {
			t.Errorf("%s: expected rejection, got nil error", h.desc)
		}
	}
}

func TestResolveTerraformPlan_ArgvAndDenials(t *testing.T) {
	reg := mustRegistry(t)
	cmd, err := reg.ResolveTerraformPlan("staging", map[string]string{
		"node_type":      "e2-small",
		"instance_count": "3",
	}, false)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := []string{
		"-chdir=./infra/staging", "plan", "-input=false", "-no-color", "-lock-timeout=30s",
		"-var=instance_count=3", "-var=node_type=e2-small", // sorted by name
	}
	if cmd.Binary != "terraform" || !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("argv mismatch:\n got %q %q\nwant terraform %q", cmd.Binary, cmd.Args, want)
	}
	if cmd.ApplyClass {
		t.Error("terraform plan is read-class; apply does not exist until Phase 2")
	}

	if _, err := reg.ResolveTerraformPlan("staging", map[string]string{"admin_password": "x"}, false); err == nil {
		t.Error("variable not in allowlist must be rejected")
	}
	if _, err := reg.ResolveTerraformPlan("staging", map[string]string{"node_type": "a\nb"}, false); err == nil {
		t.Error("variable value with newline must be rejected")
	}
	if _, err := reg.ResolveTerraformPlan("production", nil, false); err == nil {
		t.Error("production has no terraform config; must be rejected")
	}
}

func TestResolveTerraformSavedPlanPipeline(t *testing.T) {
	reg := mustRegistry(t)

	// plan with save=true writes the fixed per-environment plan file.
	plan, err := reg.ResolveTerraformPlan("staging", nil, true)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Args[len(plan.Args)-1] != "-out=openexec-staging.tfplan" {
		t.Errorf("save plan argv = %q", plan.Args)
	}
	if plan.ApplyClass {
		t.Error("plan (even with save) is read-class")
	}

	// show -json inspects exactly that file.
	show, err := reg.ResolveTerraformShowPlan("staging")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	wantShow := []string{"-chdir=./infra/staging", "show", "-json", "openexec-staging.tfplan"}
	if !reflect.DeepEqual(show.Args, wantShow) {
		t.Errorf("show argv = %q, want %q", show.Args, wantShow)
	}
	if show.ApplyClass {
		t.Error("show is read-class")
	}

	// apply consumes only the saved plan: no vars, no targets, no paths.
	apply, err := reg.ResolveTerraformApply("staging")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	wantApply := []string{"-chdir=./infra/staging", "apply", "-input=false", "-no-color", "-lock-timeout=30s", "openexec-staging.tfplan"}
	if !reflect.DeepEqual(apply.Args, wantApply) {
		t.Errorf("apply argv = %q, want %q", apply.Args, wantApply)
	}
	if !apply.ApplyClass {
		t.Error("apply must be apply-class")
	}

	// Environments without terraform config refuse the whole pipeline.
	if _, err := reg.ResolveTerraformShowPlan("production"); err == nil {
		t.Error("production has no terraform config; show must be rejected")
	}
	if _, err := reg.ResolveTerraformApply("production"); err == nil {
		t.Error("production has no terraform config; apply must be rejected")
	}
}

func TestRegistryEnums(t *testing.T) {
	reg := mustRegistry(t)
	if got := reg.Environments(); !reflect.DeepEqual(got, []string{"production", "staging"}) {
		t.Errorf("Environments() = %v", got)
	}
	if got := reg.Playbooks(); !reflect.DeepEqual(got, []string{"deploy_staging.yml", "rolling_restart.yml"}) {
		t.Errorf("Playbooks() = %v", got)
	}
	if got := reg.States(); !reflect.DeepEqual(got, []string{"app.deploy", "nginx.setup"}) {
		t.Errorf("States() = %v", got)
	}
	if got := reg.Queries(); !reflect.DeepEqual(got, []string{"check_disk", "check_service"}) {
		t.Errorf("Queries() = %v", got)
	}
	if !reg.HasEngine("ansible") || !reg.HasEngine("ssh") {
		t.Error("HasEngine should report configured engines")
	}
	if reg.HasEngine("kubectl") {
		t.Error("HasEngine must not report unknown engines")
	}
}

func TestNewRegistry_RejectsDisabledAndInvalid(t *testing.T) {
	if _, err := NewRegistry(nil); err == nil {
		t.Error("nil config must be rejected")
	}
	cfg := testConfig()
	cfg.Enabled = false
	if _, err := NewRegistry(cfg); err == nil {
		t.Error("disabled config must be rejected")
	}
	bad := testConfig()
	bad.Environments["staging"].Ansible.Playbooks = []string{"../escape.yml"}
	if _, err := NewRegistry(bad); err == nil || !strings.Contains(err.Error(), "basename") {
		t.Errorf("path-traversal playbook in config must fail validation, got %v", err)
	}
	badRisk := testConfig()
	badRisk.Environments["staging"].RiskProfile = "medium"
	if _, err := NewRegistry(badRisk); err == nil {
		t.Error("unknown risk_profile must fail validation")
	}
}
