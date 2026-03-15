// Package policy provides a rules-based policy engine for tool authorization.
// This replaces the simple keyword matching with structured rule evaluation.
package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Decision represents the outcome of policy evaluation.
type Decision string

const (
	DecisionAllow Decision = "allow" // Proceed without user interaction
	DecisionAsk   Decision = "ask"   // Prompt user for confirmation
	DecisionDeny  Decision = "deny"  // Block the action
)

// Rule represents a single policy rule.
type Rule struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Decision    Decision     `json:"decision"`
	Priority    int          `json:"priority,omitempty"`    // Higher priority rules evaluated first
	Conditions  []Condition  `json:"conditions,omitempty"`  // All must match (AND)
	Exceptions  []Condition  `json:"exceptions,omitempty"`  // If any match, rule doesn't apply
}

// Condition represents a single condition in a rule.
type Condition struct {
	Type     string `json:"type"`                // tool, path, tier, user, pattern, env
	Operator string `json:"operator,omitempty"`  // eq, ne, contains, matches, in, prefix, suffix
	Value    string `json:"value"`               // value to compare against
	Values   []string `json:"values,omitempty"`  // for "in" operator
}

// RuleSet is a collection of rules with metadata.
type RuleSet struct {
	Version     string `json:"version"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Rules       []Rule `json:"rules"`
	DefaultDecision Decision `json:"default_decision,omitempty"` // defaults to "allow"
}

// EvaluationContext contains all information needed to evaluate a rule.
type EvaluationContext struct {
	Tool       string            // Tool name being invoked
	Path       string            // File path being accessed (if applicable)
	Action     string            // Action being performed
	Tier       string            // Permission tier (read-only, workspace-write, danger-full-access)
	User       string            // User identifier (optional)
	Env        map[string]string // Environment variables
	Args       map[string]interface{} // Tool arguments
}

// EvaluationResult contains the result of policy evaluation.
type EvaluationResult struct {
	Decision    Decision `json:"decision"`
	MatchedRule *Rule    `json:"matched_rule,omitempty"`
	Reason      string   `json:"reason"`
}

// RulesEngine evaluates tool actions against policy rules.
type RulesEngine struct {
	ruleSet *RuleSet
	compiled map[string]*regexp.Regexp // Cache for compiled patterns
}

// NewRulesEngine creates a new rules engine with the given rule set.
func NewRulesEngine(ruleSet *RuleSet) *RulesEngine {
	if ruleSet.DefaultDecision == "" {
		ruleSet.DefaultDecision = DecisionAllow
	}
	return &RulesEngine{
		ruleSet:  ruleSet,
		compiled: make(map[string]*regexp.Regexp),
	}
}

// LoadRulesFromFile loads a rule set from a JSON file.
func LoadRulesFromFile(path string) (*RuleSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read rules file: %w", err)
	}
	return ParseRules(data)
}

// ParseRules parses a JSON rule set.
func ParseRules(data []byte) (*RuleSet, error) {
	var ruleSet RuleSet
	if err := json.Unmarshal(data, &ruleSet); err != nil {
		return nil, fmt.Errorf("failed to parse rules: %w", err)
	}
	if err := ruleSet.Validate(); err != nil {
		return nil, err
	}
	return &ruleSet, nil
}

// Validate checks the rule set for errors.
func (rs *RuleSet) Validate() error {
	validDecisions := map[Decision]bool{
		DecisionAllow: true,
		DecisionAsk:   true,
		DecisionDeny:  true,
	}

	if rs.DefaultDecision != "" && !validDecisions[rs.DefaultDecision] {
		return fmt.Errorf("invalid default_decision: %s", rs.DefaultDecision)
	}

	for i, rule := range rs.Rules {
		if rule.ID == "" {
			return fmt.Errorf("rule %d: id is required", i)
		}
		if !validDecisions[rule.Decision] {
			return fmt.Errorf("rule %s: invalid decision: %s", rule.ID, rule.Decision)
		}
		for j, cond := range rule.Conditions {
			if err := cond.Validate(); err != nil {
				return fmt.Errorf("rule %s condition %d: %w", rule.ID, j, err)
			}
		}
		for j, exc := range rule.Exceptions {
			if err := exc.Validate(); err != nil {
				return fmt.Errorf("rule %s exception %d: %w", rule.ID, j, err)
			}
		}
	}
	return nil
}

// Validate checks a condition for errors.
func (c *Condition) Validate() error {
	validTypes := map[string]bool{
		"tool": true, "path": true, "tier": true,
		"user": true, "pattern": true, "env": true,
		"action": true, "arg": true,
	}
	if !validTypes[c.Type] {
		return fmt.Errorf("invalid condition type: %s", c.Type)
	}

	validOperators := map[string]bool{
		"": true, "eq": true, "ne": true, "contains": true,
		"matches": true, "in": true, "prefix": true, "suffix": true,
	}
	if !validOperators[c.Operator] {
		return fmt.Errorf("invalid operator: %s", c.Operator)
	}

	if c.Operator == "in" && len(c.Values) == 0 {
		return fmt.Errorf("'in' operator requires values array")
	}

	return nil
}

// Evaluate evaluates the policy against the given context.
func (e *RulesEngine) Evaluate(ctx *EvaluationContext) *EvaluationResult {
	// Sort rules by priority (higher first)
	rules := make([]Rule, len(e.ruleSet.Rules))
	copy(rules, e.ruleSet.Rules)
	sortRulesByPriority(rules)

	for _, rule := range rules {
		// Check if rule applies
		if e.matchesConditions(ctx, rule.Conditions) {
			// Check for exceptions
			if len(rule.Exceptions) > 0 && e.matchesAny(ctx, rule.Exceptions) {
				continue // Exception matched, skip this rule
			}

			return &EvaluationResult{
				Decision:    rule.Decision,
				MatchedRule: &rule,
				Reason:      fmt.Sprintf("matched rule: %s", rule.Name),
			}
		}
	}

	// No rule matched, use default
	return &EvaluationResult{
		Decision: e.ruleSet.DefaultDecision,
		Reason:   "no matching rule; using default policy",
	}
}

// matchesConditions checks if all conditions match (AND logic).
func (e *RulesEngine) matchesConditions(ctx *EvaluationContext, conditions []Condition) bool {
	if len(conditions) == 0 {
		return true // No conditions = always matches
	}
	for _, cond := range conditions {
		if !e.matchesCondition(ctx, &cond) {
			return false
		}
	}
	return true
}

// matchesAny checks if any condition matches (OR logic for exceptions).
func (e *RulesEngine) matchesAny(ctx *EvaluationContext, conditions []Condition) bool {
	for _, cond := range conditions {
		if e.matchesCondition(ctx, &cond) {
			return true
		}
	}
	return false
}

// matchesCondition evaluates a single condition.
func (e *RulesEngine) matchesCondition(ctx *EvaluationContext, cond *Condition) bool {
	var value string
	switch cond.Type {
	case "tool":
		value = ctx.Tool
	case "path":
		value = ctx.Path
	case "tier":
		value = ctx.Tier
	case "user":
		value = ctx.User
	case "action":
		value = ctx.Action
	case "env":
		// env:VAR_NAME format
		parts := strings.SplitN(cond.Value, ":", 2)
		if len(parts) == 2 && ctx.Env != nil {
			value = ctx.Env[parts[0]]
			cond = &Condition{
				Type:     "env",
				Operator: cond.Operator,
				Value:    parts[1],
				Values:   cond.Values,
			}
		} else {
			return false
		}
	case "arg":
		// arg:name format
		parts := strings.SplitN(cond.Value, ":", 2)
		if len(parts) == 2 && ctx.Args != nil {
			if v, ok := ctx.Args[parts[0]]; ok {
				value = fmt.Sprintf("%v", v)
				cond = &Condition{
					Type:     "arg",
					Operator: cond.Operator,
					Value:    parts[1],
					Values:   cond.Values,
				}
			}
		} else {
			return false
		}
	default:
		return false
	}

	return e.evaluateOperator(value, cond)
}

// evaluateOperator applies the operator to compare values.
func (e *RulesEngine) evaluateOperator(value string, cond *Condition) bool {
	op := cond.Operator
	if op == "" {
		op = "eq" // Default operator
	}

	switch op {
	case "eq":
		return value == cond.Value
	case "ne":
		return value != cond.Value
	case "contains":
		return strings.Contains(value, cond.Value)
	case "prefix":
		return strings.HasPrefix(value, cond.Value)
	case "suffix":
		return strings.HasSuffix(value, cond.Value)
	case "in":
		for _, v := range cond.Values {
			if value == v {
				return true
			}
		}
		return false
	case "matches":
		re, err := e.getCompiledRegex(cond.Value)
		if err != nil {
			return false
		}
		return re.MatchString(value)
	default:
		return false
	}
}

// getCompiledRegex returns a cached compiled regex.
func (e *RulesEngine) getCompiledRegex(pattern string) (*regexp.Regexp, error) {
	if re, ok := e.compiled[pattern]; ok {
		return re, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	e.compiled[pattern] = re
	return re, nil
}

// sortRulesByPriority sorts rules by priority (descending).
func sortRulesByPriority(rules []Rule) {
	for i := 0; i < len(rules); i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[j].Priority > rules[i].Priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}
}

// DefaultSecurityRules returns a sensible default rule set for security.
func DefaultSecurityRules() *RuleSet {
	return &RuleSet{
		Version:         "1.0",
		Name:            "OpenExec Default Security Policy",
		Description:     "Built-in security rules for safe operation",
		DefaultDecision: DecisionAllow,
		Rules: []Rule{
			{
				ID:       "deny-etc-write",
				Name:     "Block writes to /etc",
				Priority: 100,
				Decision: DecisionDeny,
				Conditions: []Condition{
					{Type: "tool", Operator: "in", Values: []string{"write_file", "run_shell_command"}},
					{Type: "path", Operator: "prefix", Value: "/etc/"},
				},
			},
			{
				ID:       "deny-passwd-shadow",
				Name:     "Block access to passwd/shadow",
				Priority: 100,
				Decision: DecisionDeny,
				Conditions: []Condition{
					{Type: "path", Operator: "matches", Value: `/(etc/)?(passwd|shadow|sudoers)`},
				},
			},
			{
				ID:       "ask-force-push",
				Name:     "Confirm force push",
				Priority: 90,
				Decision: DecisionAsk,
				Conditions: []Condition{
					{Type: "tool", Value: "run_shell_command"},
					{Type: "action", Operator: "contains", Value: "push"},
					{Type: "action", Operator: "contains", Value: "--force"},
				},
			},
			{
				ID:       "ask-delete-main",
				Name:     "Confirm delete on main branch",
				Priority: 90,
				Decision: DecisionAsk,
				Conditions: []Condition{
					{Type: "tool", Value: "run_shell_command"},
					{Type: "action", Operator: "matches", Value: `git\s+(branch\s+-[dD]|push\s+.*:\s*).*main`},
				},
			},
			{
				ID:       "deny-rm-rf-root",
				Name:     "Block recursive delete at root",
				Priority: 100,
				Decision: DecisionDeny,
				Conditions: []Condition{
					{Type: "tool", Value: "run_shell_command"},
					{Type: "action", Operator: "matches", Value: `rm\s+-rf?\s+/\s*$`},
				},
			},
			{
				ID:       "deny-credentials",
				Name:     "Block access to credentials files",
				Priority: 100,
				Decision: DecisionDeny,
				Conditions: []Condition{
					{Type: "path", Operator: "matches", Value: `\.(env|credentials|secrets?|key|pem)$`},
				},
				Exceptions: []Condition{
					{Type: "path", Operator: "suffix", Value: ".example"},
				},
			},
			{
				ID:       "allow-readonly-tier",
				Name:     "Allow all in read-only tier",
				Priority: 50,
				Decision: DecisionAllow,
				Conditions: []Condition{
					{Type: "tier", Value: "read-only"},
					{Type: "tool", Operator: "in", Values: []string{"read_file", "list_files", "search"}},
				},
			},
			{
				ID:       "ask-workspace-write",
				Name:     "Confirm writes in workspace tier",
				Priority: 40,
				Decision: DecisionAsk,
				Conditions: []Condition{
					{Type: "tier", Value: "workspace-write"},
					{Type: "tool", Operator: "in", Values: []string{"write_file", "git_apply_patch"}},
				},
			},
		},
	}
}

// LoadRulesFromProject loads rules from .openexec/policy.json if it exists.
func LoadRulesFromProject(projectDir string) (*RuleSet, error) {
	policyPath := filepath.Join(projectDir, ".openexec", "policy.json")
	if _, err := os.Stat(policyPath); os.IsNotExist(err) {
		return DefaultSecurityRules(), nil
	}
	return LoadRulesFromFile(policyPath)
}
