package prompt

import (
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/release"
)

// FormatBriefing transforms a BriefResponse into structured prompt text
// suitable for injection into an agent's system prompt.
func FormatBriefing(brief *release.BriefResponse) string {
	var b strings.Builder

	// Header
	fmt.Fprintf(&b, "## FWU Briefing: %s — %s\n\n", brief.FWU.ID, brief.FWU.Name)
	fmt.Fprintf(&b, "**Status:** %s\n", brief.FWU.Status)
	fmt.Fprintf(&b, "**Intent:** %s\n", brief.FWU.Intent)

	// Boundaries
	writeBoundaries(&b, brief.Boundaries)

	// Dependencies
	writeDependencies(&b, brief.Dependencies, brief.DependencyStatus)

	// Design Decisions
	writeDesignDecisions(&b, brief.DesignDecisions)

	// Interface Contracts
	writeInterfaceContracts(&b, brief.InterfaceContracts)

	// Verification Gates
	writeVerificationGates(&b, brief.VerificationGates)

	// Reasoning Chain
	writeReasoningChain(&b, brief.ReasoningChain)

	// Predecessor Specs
	writePredecessorSpecs(&b, brief.PredecessorSpecs)

	// Prior ICs
	writePriorICs(&b, brief.PriorICs, brief.PriorICCount)

	return b.String()
}

func writeBoundaries(b *strings.Builder, boundaries []release.Boundary) {
	if len(boundaries) == 0 {
		return
	}

	var inScope, outOfScope []release.Boundary
	for _, bn := range boundaries {
		switch bn.Scope {
		case "in_scope":
			inScope = append(inScope, bn)
		case "out_of_scope":
			outOfScope = append(outOfScope, bn)
		}
	}

	b.WriteString("\n### Boundaries\n")

	if len(inScope) > 0 {
		b.WriteString("\n**In scope:**\n")
		for _, bn := range inScope {
			fmt.Fprintf(b, "- [%s] %s\n", bn.ID, bn.Description)
		}
	}

	if len(outOfScope) > 0 {
		b.WriteString("\n**Out of scope:**\n")
		for _, bn := range outOfScope {
			fmt.Fprintf(b, "- [%s] %s\n", bn.ID, bn.Description)
		}
	}
}

func writeDependencies(b *strings.Builder, deps []release.Dependency, statuses []release.DependencyStatus) {
	if len(deps) == 0 {
		return
	}

	statusMap := make(map[string]release.DependencyStatus, len(statuses))
	for _, ds := range statuses {
		statusMap[ds.DependencyID] = ds
	}

	b.WriteString("\n### Dependencies\n\n")
	for _, dep := range deps {
		fmt.Fprintf(b, "- [%s] (%s) %s: %s\n", dep.ID, dep.DependencyType, dep.TargetFWUID, dep.Description)
		if ds, ok := statusMap[dep.ID]; ok {
			fmt.Fprintf(b, "  Status: %s — %s \"%s\"\n", ds.TargetStatus, ds.TargetFWUID, ds.TargetFWUName)
		}
	}
}

func writeDesignDecisions(b *strings.Builder, decisions []release.DesignDecision) {
	if len(decisions) == 0 {
		return
	}

	b.WriteString("\n### Design Decisions\n\n")
	for _, dd := range decisions {
		fmt.Fprintf(b, "- [%s] **%s** → %s\n", dd.ID, dd.Decision, dd.Resolution)
		if dd.Rationale != "" {
			fmt.Fprintf(b, "  Rationale: %s\n", dd.Rationale)
		}
	}
}

func writeInterfaceContracts(b *strings.Builder, contracts []release.InterfaceContract) {
	if len(contracts) == 0 {
		return
	}

	b.WriteString("\n### Interface Contracts\n\n")
	for _, ct := range contracts {
		fmt.Fprintf(b, "- [%s] (%s → %s) %s\n", ct.ID, ct.Direction, ct.CounterpartFWUID, ct.Description)
	}
}

func writeVerificationGates(b *strings.Builder, gates []release.VerificationGate) {
	if len(gates) == 0 {
		return
	}

	b.WriteString("\n### Verification Gates\n\n")
	for _, vg := range gates {
		fmt.Fprintf(b, "- [%s] %s: %s\n", vg.ID, vg.Gate, vg.Expectation)
	}
}

func writeReasoningChain(b *strings.Builder, chain *release.ReasoningChain) {
	if chain == nil {
		return
	}

	b.WriteString("\n### Reasoning Chain\n\n")

	var parts []string

	for _, g := range chain.Goals {
		parts = append(parts, fmt.Sprintf("Goal: %s \"%s\"", g.ID, g.Name))
	}
	for _, csf := range chain.CSFs {
		parts = append(parts, fmt.Sprintf("CSF: %s \"%s\"", csf.ID, csf.Name))
	}
	for _, nc := range chain.NCs {
		parts = append(parts, fmt.Sprintf("NC: %s \"%s\"", nc.ID, nc.Name))
	}
	if chain.SO != nil {
		parts = append(parts, fmt.Sprintf("SO: %s \"%s\"", chain.SO.ID, chain.SO.Name))
	}
	if chain.Capability != nil {
		parts = append(parts, fmt.Sprintf("Capability: %s \"%s\"", chain.Capability.ID, chain.Capability.Name))
	}
	if chain.Epic != nil {
		parts = append(parts, fmt.Sprintf("Epic: %s \"%s\"", chain.Epic.ID, chain.Epic.Name))
	}
	if chain.Feature != nil {
		parts = append(parts, fmt.Sprintf("Feature: %s \"%s\"", chain.Feature.ID, chain.Feature.Name))
	}

	if len(parts) > 0 {
		b.WriteString(strings.Join(parts, " → "))
		b.WriteString("\n")
	}
}

func writePredecessorSpecs(b *strings.Builder, specs []release.PredecessorSpec) {
	if len(specs) == 0 {
		return
	}

	b.WriteString("\n### Predecessor Specs\n\n")
	for _, ps := range specs {
		fmt.Fprintf(b, "From %s (%s):\n", ps.SourceFWUID, ps.SourceICID)
		fmt.Fprintf(b, "- %s (%s)", ps.EntityName, ps.EntityType)
		if ps.ParentClass != "" {
			fmt.Fprintf(b, " in %s", ps.ParentClass)
		}
		if ps.CodeBlock != "" {
			fmt.Fprintf(b, ": `%s`", ps.CodeBlock)
		}
		b.WriteString("\n")
	}
}

func writePriorICs(b *strings.Builder, priorICs []release.PriorIC, count int) {
	if len(priorICs) == 0 {
		return
	}

	b.WriteString("\n### Prior Implementation Contexts\n\n")
	fmt.Fprintf(b, "%d prior attempt(s):\n", count)
	for _, ic := range priorICs {
		fmt.Fprintf(b, "- %s (attempt %d): %s, planning v%d\n", ic.ICID, ic.Attempt, ic.Status, ic.PlanningVersion)
	}
}
