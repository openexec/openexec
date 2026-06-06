# Roadmap: BitNet-Gated SRE & Infrastructure Orchestration (Salt, Ansible, SSH, Terraform)

This document provides detailed implementation instructions and technical specifications for building a non-destructive, bulletproof SRE and deployment orchestration layer inside OpenExec. 

**Core Design Philosophy:** Safety must be absolute, deterministic, and model-independent. The AI's action space is constrained at the schema level so that destructive verbs cannot be hallucinated, and execution bypasses shell expansions entirely.

---

## Phase 1: Config-Driven SRE Command Registry & Go Executor

### Purpose
To eliminate raw terminal/shell access (`run_shell_command`) and replace it with a **deny-by-default, allowlist-only** registry of specific, parameter-bounded tools. This ensures that the AI cannot execute arbitrary, untested, or destructive command strings.

### Proposed Implementation

#### 1. Configuration Schema (`openexec.yaml` / `config.json`)
Introduce a new `infrastructure_orchestration` section defining allowed playbooks, states, and SSH queries per environment:

```yaml
infrastructure_orchestration:
  enabled: true
  environments:
    staging:
      risk_profile: low
      allowlist:
        terraform:
          working_dir: "./infra/staging"
          allowed_variables: ["instance_count", "node_type"]
        ansible:
          playbooks: ["deploy_staging.yml", "rolling_restart.yml"]
        salt:
          states: ["nginx.setup", "app.deploy"]
        ssh:
          allowed_hosts: ["10.0.1.*"]
          allowed_queries: ["check_disk", "check_service"]
    production:
      risk_profile: high  # Forces HITL / approval gate on all apply actions
      allowlist:
        terraform:
          working_dir: "./infra/prod"
          allowed_variables: ["instance_count"]
        ansible:
          playbooks: ["rolling_restart.yml"]
        ssh:
          allowed_hosts: ["10.0.2.*"]
          allowed_queries: ["check_service"]
```

#### 2. The Go Command Executor (`internal/toolset/sre_executor.go`)
Implement a strict command builder that constructs slice-based argument arrays for `exec.CommandContext`. **Never use shell expansion (`sh -c`).**

```go
package toolset

import (
	"context"
	"fmt"
	"os/exec"
)

// ExecuteAnsible runs a pre-approved playbook with strict argument boundaries.
// The playbook parameter is a BASENAME from the config enum (never a caller-supplied
// path — that would invite ../ traversal); it is joined against the configured
// working dir only after the enum check passes.
func ExecuteAnsible(ctx context.Context, playbook string, inventory string, limit string) (string, error) {
	// 1. Validate against allowlist (exact enum match on basename)
	if !isPlaybookAllowed(playbook) {
		return "", fmt.Errorf("playbook %s is not in the active allowlist", playbook)
	}
	playbookPath := filepath.Join(cfg.WorkingDir, playbook)

	// 2. Build argument array strictly — NO string interpolation/shell evaluation.
	//    Validate-and-REJECT, never sanitize: hostile input is refused, not transformed.
	args := []string{"-i", inventory, playbookPath}
	if limit != "" {
		if !limitPatternRe.MatchString(limit) { // e.g. ^[a-zA-Z0-9_.:*-]+$
			return "", fmt.Errorf("limit %q does not match the allowed pattern", limit)
		}
		args = append(args, "--limit", limit)
	}

	cmd := exec.CommandContext(ctx, "/usr/bin/ansible-playbook", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
```

#### 3. Dynamic MCP Tool Compiler (`internal/mcp/sre_tools.go`)
Read the allowlist config at server boot time and dynamically compile allowed commands into specific, parameter-bounded MCP tool schemas.
*   Instead of `run_shell_command`, the model sees `terraform_plan`, `ansible_run_playbook`, and `ssh_query_status` with strict parameter drop-downs (JSON Enums) mapped directly to your config.

---

## Phase 2: Saved-Plan (TOCTOU-Free) Blueprint & Deterministic JSON Plan Gating

### Purpose
To eliminate the **Time-of-Check to Time-of-Use (TOCTOU)** vulnerability where cluster/infra state drifts or files are mutated between the planning and applying phases. This phase also replaces fuzzy "LLM code reviews" of deployment plans with deterministic JSON policy gates.

### Proposed Implementation

#### 1. The Saved-Plan Pipeline
Design an OpenExec Blueprint that strictly separates planning and applying, forcing the consumption of a binary plan file:

```
[ Step 1: PLAN ] ────────► Generates binary plan: `deploy.tfplan`
                                 │
                                 ▼
[ Step 2: VERIFY ] ──────► Parses plan JSON deterministically (OPA / struct-check)
                                 │
                                 ▼
[ Step 3: APPLY ] ───────► Runs `terraform apply deploy.tfplan` (No fresh planning)
```

#### 2. Deterministic JSON Plan Verification
Instead of feeding the text plan output back to an LLM for safety validation, execute `terraform show -json deploy.tfplan` and parse the structurally typed modifications.

```go
type TFPlan struct {
	ResourceChanges []struct {
		Address string `json:"address"`
		Change  struct {
			Actions []string `json:"actions"` // ["create"], ["read"], ["update"], ["delete"]
		} `json:"change"`
	} `json:"resource_changes"`
}

func VerifyTFPlan(planJSON []byte) (bool, []string) {
	var plan TFPlan
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return false, []string{"malformed plan json"}
	}

	// NOTE: Terraform's JSON plan never emits a literal "replace" action.
	// Replacement is encoded as the action PAIR ["delete","create"] (or
	// ["create","delete"] with create_before_destroy). Checking for "delete"
	// therefore catches both pure deletions and all replacements.
	var destructiveChanges []string
	for _, change := range plan.ResourceChanges {
		for _, action := range change.Change.Actions {
			if action == "delete" {
				kind := "delete"
				if len(change.Change.Actions) > 1 { // delete paired with create = replace
					kind = "replace"
				}
				destructiveChanges = append(destructiveChanges, fmt.Sprintf("Destructive action [%s] detected on resource: %s", kind, change.Address))
			}
		}
	}
	
	// If destructiveChanges list is not empty, block execution and halt for approval
	return len(destructiveChanges) == 0, destructiveChanges
}
```

#### 3. State-Machine "Abort"
If the plan verification fails (e.g., a database replacement is detected), the Go pipeline:
1.  Blocks the transition to the `apply` stage.
2.  Raises an `openexec_signal` with type `decision-point` containing the JSON-parsed violations.
3.  Halts the run, awaiting explicit operator sign-off.

---

## Phase 3: Risk-Tiered Environment Policy, HITL Sign-off & Audit Trails

### Purpose
To establish strict operational boundaries between staging and production environments, providing a clean, human-in-the-loop (HITL) manual override system over standard stdio streams, backed by immutable audit histories.

### Proposed Implementation

#### 1. Environment-Aware Permission Broker (`internal/mcp/broker.go`)
Extend the permission broker to gate tools dynamically based on target environment parameters:
*   **Staging:** Configured as `RiskLow`. Commands can execute fully autonomously (`AFK-able`).
*   **Production:** Configured as `RiskHigh`. Every write or apply-class action (e.g. `ansible_run_playbook` with prod inventory, `terraform_apply`) is marked as `RequiresApproval = true` and `HITL = true`.

#### 2. Stdio Backlog Sign-off via the Two-Speed Seam
When an apply action is blocked on a production environment:
1.  The heavy daemon halts, writes the blocked state, the generated `.tfplan` diff, and the verification metrics into `.openexec/openexec.db`.
2.  The task surfaces as a pending story on the backlog.
3.  The SRE operator launches Claude Code (light mode) via the stdio `mcp-serve` connection:
    *   Runs `backlog_get_story` to inspect the plan diff and the block reason.
    *   Signs off via a **dedicated `approve_action` tool** wired to `internal/approval`.
        **Never reuse `backlog_complete_task` as the approval signal**: backlog tools are
        deliberately allowed in *all* permission modes including read-only chat (documented
        exception — they mutate orchestrator bookkeeping, not the workspace). If completing
        a task could trigger a production apply, that exception becomes an authorization
        bypass. `approve_action` must be gated by the permission broker like any
        high-risk tool.
4.  The background runner receives the database update, unlocks the state, and applies the saved plan.

> **Prerequisite work item:** `internal/approval` today holds pending requests in memory
> with a 5-minute `DefaultTimeout` (`gate.go`). Hours-async sign-off requires persisting
> pending approvals to SQLite (open via `pkg/db/sqlitecfg.DSN`) so a blocked apply
> survives daemon restarts and waits indefinitely (or until an explicit expiry).

#### 3. Immutable SRE Audit Logging (`pkg/audit/`)
Every compiled command execution, variable parameter, run stdout/stderr, and human approval signature is written async to the SQLite-backed `audit_entries` table with SHA256 integrity hashes. This ensures full traceability for public-sector and enterprise compliance audits.

---

## Phase 4: Intent Routing & Optional Local BitNet Router

### Purpose
To add a natural-language routing layer that translates developer prompts (e.g., *"bounce the nginx servers on staging"*) into the correct, safe, parameter-bounded tool calls from the registry.

### Proposed Implementation

#### 1. The Heuristic Router (DCP Selector)
Before deploying any complex LLM at the security boundary, utilize OpenExec's deterministic keyword/synonym selector (`internal/dcp/selector.go`):
*   Map SRE verbs (e.g., "deploy", "restart", "upgrade") and environments to specific tool IDs.
*   **Safety Guarantee:** If the heuristic classifier falls below the router's `LowConfidenceThreshold` (`internal/dcp/coordinator.go` — configurable; set it high for the SRE lane), route the task immediately to `general_chat` or raise a `scope-discovery` signal to the user for clarification.

#### 2. Local BitNet LLM Integration
As an optional optimization pass:
*   Configure a local, quantized, 1-bit LLM (such as a 1.5B or 7B Qwen/Llama GGUF) running locally on the operator's machine or as a lightweight sidecar in the VPC.
*   The model receives the developer's raw prompt and outputs a structured classification mapping (e.g., `{"tool": "ansible_run_playbook", "args": {"playbook": "rolling_restart.yml", "limit": "nginx"}}`).
*   **Safety Boundary:** The BitNet output is treated strictly as an *untrusted proposal*. It is fed into the Go `CommandRegistry` compiler, which validates that the proposed arguments conform perfectly to your whitelist and regex checks before spawning any process.

---

## Preventing Tool Bypasses & Schema Migration Escapes

### The Threat Scenario
An agent is tasked with deploying a schema migration. Instead of writing and running a pre-approved, automated migration script (or because it gets confused by a database error), the agent:
- Tries to run raw SSH commands directly on the production DB host to manually modify tables.
- Tries to bypass Terraform by calling `gcloud sql databases delete` or direct Cloud CLI commands.
- Forgets the migration step entirely and deploys code that causes DB schema misalignment or data loss.

To mathematically eliminate this "Execution Escape" vector, OpenExec enforces four strict system-level security boundaries:

### 1. Zero Ambient Admin Credentials (Credential Sandboxing)
Never run OpenExec's execution runner inside an environment that has master, administrator-level cloud credentials (`gcloud`, `aws`, `kubectl`) or unscoped SSH private keys loaded in its environment variables.
*   **The Guardrail:** The runner process should only hold a highly scoped service account token that *only* has permission to read/write specific states, view configurations, or trigger Salt/Ansible targets.
*   **Why it works:** If the LLM somehow attempts to bypass Terraform and run a raw `gcloud sql databases delete` command, the Cloud Provider's IAM API will instantly reject the call with a `403 Forbidden` because OpenExec's token lacks administrative delete privileges.

### 2. Total Toolset Pruning (No Generic Shell Access)
When OpenExec is executing an infrastructure or SRE deployment blueprint, **the generic `run_shell_command` tool is completely removed from the model's active tool list.**
*   **The Guardrail:** The model's available tool schema is restricted strictly to: `terraform_plan`, `terraform_apply`, `ansible_run_playbook`, and `ssh_query_status`.
*   **Why it works:** Because there is no general-purpose command-line tool exposed in the active MCP schema, **the model physically has no "mouth" to invoke raw `gcloud` or `ssh` commands.** It is structurally locked within the boundaries of your compiled command registry.

### 3. Pre-Flight Migration Integrity Gates (Go-based)
To prevent the model from "forgetting" to run migration scripts and deploying misaligned code:
*   **The Guardrail:** The Go-based deployment blueprint contains a mandatory **Pre-Flight Migration Gate** (Step 0) executed before any Terraform or Ansible apply.
*   **The Logic:** The Go gate parses the local Git diff. If it detects changes to database entities (e.g., entity models, schema definitions) *without* finding a corresponding new migration script appended to your pre-approved `migrations/` folder, the Go pipeline **instantly blocks the deployment** and raises a `decision-point` signal.
*   **Why it works:** The AI cannot "forget" to run migrations because a deterministic Go parser verifies that migration scripts exist and are registered before letting the deployment start.

### 4. Host-Level Command Scoping (SSH Restrictions)
Any SSH keys provisioned for OpenExec to access staging or production hosts must be locked down at the target host level, rather than granting full shell access.
*   **The Guardrail:** Configure target host SSH directories (`.ssh/authorized_keys`) to force command-scoping:
    ```bash
    command="/usr/bin/safe_diagnostic_script",no-port-forwarding,no-x11-forwarding ssh-rsa AAAAB3NzaC1y...
    ```
*   **Why it works:** Even if an agent gets access to the SSH private key, any connection attempt to the target DB host is forced by the host's SSH daemon to *only* execute that single pre-approved, safe diagnostic script—making manual database dropping or table modification over SSH physically impossible.

---

## Neutralizing Indirect Payload Generation & Script Escapes

### The Threat Scenario
You are completely restricting raw terminal commands, but the AI still has access to file editing tools (like `write_file` or `git_apply_patch`). 
To bypass your command allowlist, the AI writes a highly destructive script to a temporary file (e.g. `/tmp/deploy.sh` containing `rm -rf /` or database drop statements) and then uses a seemingly "harmless" whitelisted tool (or an allowed playbook/script executor) to trigger the execution of that local script file. 

This is an **Indirect Payload Generation (Privilege Escalation)** exploit. To mathematically neutralize this vector, OpenExec implements three structural safety barriers:

### 1. Absolute Write-Run Tool Separation (No Generic Interpreters)
If an agent can write files, but **the execution environment lacks any generic interpreter tool**, it cannot execute the script.
*   **The Guardrail:** During infrastructure-touching runs, we never expose `run_shell_command` or any generic script runner.
*   **Why it works:** The AI can write 100 malicious scripts to `/tmp/deploy.sh`, but if the only tools it is given are `terraform_plan`, `terraform_apply`, and `ansible_run_playbook`, **none of these tools accept an arbitrary file path to execute.** 
    - `terraform_apply` only accepts a compiled, binary `.tfplan` file.
    - `ansible_run_playbook` only executes playbooks from your audited config list.
    The AI is left with a collection of malicious text files that it has no physical mechanism to execute.

### 2. Read-Only Git Allowlists for Playbooks and States
An agent might try to modify a pre-approved Ansible playbook in the local workspace (e.g. appending a malicious shell task to `deploy_staging.yml`) and then execute it via the whitelisted `ansible_run_playbook` tool.
*   **The Guardrail:** The Go-based `ansible_run_playbook` executor must **strictly checkout and execute playbooks from a read-only Git commit reference, or read them from a read-only system directory** (e.g., `/etc/openexec/playbooks/`).
*   **Why it works:** Even if the agent modifies `deploy_staging.yml` in its local workspace, the Go backend completely ignores those local changes during execution. It pulls the pristine, signed `main` branch version of that playbook from Git or read-only storage, neutralizing the modified payload entirely.

### 3. File System `noexec` Sandboxing
If OpenExec is executing infrastructure actions locally, we can enforce operating system-level restrictions on script execution.
*   **The Guardrail:** Mount the local temporary directory (`/tmp/`) and the agent's active workspace directory with the **`noexec` mount option** (or run inside a highly restricted, read-only Docker container containerized sandbox).
*   **Why it works:** The operating system's kernel itself will refuse to execute any binary, shell script, or Python payload located within `/tmp/` or the workspace, throwing an immediate `Permission Denied` error even if a process attempts to trigger it.


