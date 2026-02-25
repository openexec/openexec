package prompt

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/prompt/manifest"
	"github.com/openexec/openexec/internal/prompt/persona"
	"github.com/openexec/openexec/internal/prompt/workflow"
)

// Assembler composes system prompts for agent phases using decomposed definitions.
type Assembler struct {
	personas  *persona.Store
	workflows *workflow.Store
	manifests *manifest.Store
}

// NewAssembler creates an Assembler that reads decomposed agent definitions from dir.
// dir is the root agents directory containing personas/, workflows/, manifests/ subdirs.
func NewAssembler(dir string) *Assembler {
	return &Assembler{
		personas:  persona.NewStore(filepath.Join(dir, "personas")),
		workflows: workflow.NewStore(filepath.Join(dir, "workflows")),
		manifests: manifest.NewStore(filepath.Join(dir, "manifests")),
	}
}

// Compose builds a complete system prompt for the given agent, workflow, and briefing.
// Returns the prompt string or an error if the agent, workflow, or params are invalid.
func (a *Assembler) Compose(agent, workflowID, briefing string) (string, error) {
	// Load agent manifest.
	mf, err := a.manifests.Get(agent)
	if err != nil {
		return "", fmt.Errorf("compose: %w", err)
	}

	// Load and merge persona.
	p, err := a.personas.Get(mf.Persona)
	if err != nil {
		return "", fmt.Errorf("compose: %w", err)
	}

	// Load workflow template.
	tmpl, err := a.workflows.Get(workflowID)
	if err != nil {
		return "", fmt.Errorf("compose: %w", err)
	}

	// Get param values from manifest for this workflow.
	params := mf.WorkflowParams(workflowID)

	// Validate params.
	if err := workflow.Validate(tmpl.Params, params); err != nil {
		return "", fmt.Errorf("compose: workflow %q for agent %q: %w", workflowID, agent, err)
	}

	// Expand template placeholders.
	instructions, err := workflow.Expand(tmpl.Instructions, params)
	if err != nil {
		return "", fmt.Errorf("compose: workflow %q instructions: %w", workflowID, err)
	}
	process, err := workflow.Expand(tmpl.Process, params)
	if err != nil {
		return "", fmt.Errorf("compose: workflow %q process: %w", workflowID, err)
	}

	// Build prompt — same sections and order as the original assembler.
	var b strings.Builder

	// Agent header
	fmt.Fprintf(&b, "# Agent: %s\n\n", mf.Title)

	// Persona section
	b.WriteString("## Persona\n\n")
	fmt.Fprintf(&b, "**Role:** %s\n\n", p.Role)
	fmt.Fprintf(&b, "**Identity:** %s\n\n", p.Identity)
	fmt.Fprintf(&b, "**Communication Style:** %s\n\n", p.CommunicationStyle)
	fmt.Fprintf(&b, "**Principles:** %s\n\n", p.Principles)

	// Workflow section
	b.WriteString("## Workflow\n\n")
	b.WriteString(instructions)
	b.WriteString("\n\n")
	b.WriteString(process)
	b.WriteString("\n\n")

	// Briefing (pre-formatted, may be empty)
	if briefing != "" {
		b.WriteString(briefing)
		b.WriteString("\n\n")
	}

	// Protocols
	b.WriteString(SignalProtocol())
	b.WriteString("\n\n")
	b.WriteString(ConsultProtocol())
	b.WriteString("\n")

	return b.String(), nil
}
