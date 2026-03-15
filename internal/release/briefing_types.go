package release

// BriefResponse is the structured briefing data for a task (FWU).
// This was previously part of the legacy tract package.
type BriefResponse struct {
	FWU                FWU                 `json:"fwu"`
	Boundaries         []Boundary          `json:"boundaries"`
	Dependencies       []Dependency        `json:"dependencies"`
	DesignDecisions    []DesignDecision    `json:"design_decisions"`
	InterfaceContracts []InterfaceContract `json:"interface_contracts"`
	VerificationGates  []VerificationGate  `json:"verification_gates"`
	ReasoningChain     *ReasoningChain     `json:"reasoning_chain"`
	DependencyStatus   []DependencyStatus  `json:"dependency_status"`
	PredecessorSpecs   []PredecessorSpec   `json:"predecessor_specs"`
	PriorICs           []PriorIC           `json:"prior_ics"`
	PriorICCount       int                 `json:"prior_ic_count"`
}

// FWU represents a Feature Work Unit (task) in the briefing.
type FWU struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Intent    string `json:"intent"`
	FeatureID string `json:"feature_id"`
}

// Boundary defines what is in or out of scope for a task.
type Boundary struct {
	ID          string `json:"id"`
	Scope       string `json:"scope"` // "in_scope" or "out_of_scope"
	Description string `json:"description"`
}

// Dependency represents a dependency on another task.
type Dependency struct {
	ID             string `json:"id"`
	DependencyType string `json:"dependency_type"`
	TargetFWUID    string `json:"target_fwu_id"`
	Description    string `json:"description"`
}

// DesignDecision captures a design decision with rationale.
type DesignDecision struct {
	ID         string `json:"id"`
	Decision   string `json:"decision"`
	Resolution string `json:"resolution"`
	Rationale  string `json:"rationale"`
}

// InterfaceContract defines an interface contract with another task.
type InterfaceContract struct {
	ID               string `json:"id"`
	Direction        string `json:"direction"` // "consumes" or "produces"
	CounterpartFWUID string `json:"counterpart_fwu_id"`
	Description      string `json:"description"`
}

// VerificationGate defines a verification requirement.
type VerificationGate struct {
	ID          string `json:"id"`
	Gate        string `json:"gate"` // "tests", "docs", "quality", "ops", "security"
	Expectation string `json:"expectation"`
}

// ReasoningChain traces the task back to strategic objectives.
type ReasoningChain struct {
	Feature    *ChainEntity  `json:"feature"`
	Epic       *ChainEntity  `json:"epic"`
	Capability *ChainEntity  `json:"capability"`
	SO         *ChainEntity  `json:"so"`
	NCs        []ChainEntity `json:"ncs"`
	CSFs       []ChainEntity `json:"csfs"`
	Goals      []ChainEntity `json:"goals"`
}

// ChainEntity is a node in the reasoning chain.
type ChainEntity struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// DependencyStatus tracks the status of a dependency.
type DependencyStatus struct {
	DependencyID  string `json:"dependency_id"`
	TargetFWUID   string `json:"target_fwu_id"`
	TargetFWUName string `json:"target_fwu_name"`
	TargetStatus  string `json:"target_status"`
	Description   string `json:"description"`
}

// PredecessorSpec captures specifications from predecessor tasks.
type PredecessorSpec struct {
	SourceFWUID string `json:"source_fwu_id"`
	SourceICID  string `json:"source_ic_id"`
	EntityName  string `json:"entity_name"`
	EntityType  string `json:"entity_type"`
	ParentClass string `json:"parent_class"`
	CodeBlock   string `json:"code_block"`
}

// PriorIC represents a prior implementation context attempt.
type PriorIC struct {
	ICID            string `json:"ic_id"`
	Attempt         int    `json:"attempt"`
	Status          string `json:"status"`
	PlanningVersion int    `json:"planning_version"`
}
