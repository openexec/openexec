package prompt

import (
	"fmt"
	"io/fs"
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

	lastBriefing    string
	lastWorkflowID  string
	lastAgent       string
	lastStaticBlock string
}

// NewAssembler creates an Assembler that reads decomposed agent definitions from the given filesystem.
// f is the root agents filesystem containing personas/, workflows/, manifests/ subdirs.
func NewAssembler(f fs.FS) *Assembler {
	pFS, _ := fs.Sub(f, "personas")
	wFS, _ := fs.Sub(f, "workflows")
	mFS, _ := fs.Sub(f, "manifests")

	return &Assembler{
		personas:  persona.NewStore(pFS),
		workflows: workflow.NewStore(wFS),
		manifests: manifest.NewStore(mFS),
	}
}

// Compose builds a complete system prompt for the given agent, workflow, and briefing.
// Returns the prompt string or an error if the agent, workflow, or params are invalid.
func (a *Assembler) Compose(agent, workflowID, briefing string) (string, error) {
	isContinuation := a.lastBriefing == briefing && a.lastWorkflowID == workflowID && a.lastAgent == agent
	
	var staticBlock string
	if a.lastAgent == agent && a.lastStaticBlock != "" {
		staticBlock = a.lastStaticBlock
	} else {
		// Build new static block (Agent + Persona)
		mf, err := a.manifests.Get(agent)
		if err != nil {
			return "", fmt.Errorf("compose manifest: %w", err)
		}

		p, err := a.personas.Get(mf.Persona)
		if err != nil {
			return "", fmt.Errorf("compose persona: %w", err)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "# Agent: %s\n\n", mf.Title)
		sb.WriteString("## Persona\n\n")
		fmt.Fprintf(&sb, "**Role:** %s\n\n", p.Role)
		fmt.Fprintf(&sb, "**Identity:** %s\n\n", p.Identity)
		fmt.Fprintf(&sb, "**Communication Style:** %s\n\n", p.CommunicationStyle)
		fmt.Fprintf(&sb, "**Principles:** %s\n\n", p.Principles)
		
		staticBlock = sb.String()
		a.lastStaticBlock = staticBlock
	}

	// Update cache
	a.lastBriefing = briefing
	a.lastWorkflowID = workflowID
	a.lastAgent = agent

	// Load agent manifest for workflow params
	mf, err := a.manifests.Get(agent)
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

	// Build final prompt
	var b strings.Builder
	b.WriteString(staticBlock)

	// Workflow section
	b.WriteString("## Workflow\n\n")
	b.WriteString(instructions)
	b.WriteString("\n\n")
	b.WriteString(process)
	b.WriteString("\n\n")

	// Briefing (pre-formatted, may be empty)
	if briefing != "" {
		if isContinuation {
			b.WriteString("## Briefing: CONTINUATION\n\n")
			b.WriteString("The briefing for this task remains identical to your previous phase. Maintain all existing design decisions and constraints. Focus on completing the remaining workflow steps for this same task.\n\n")
		} else {
			b.WriteString(briefing)
			b.WriteString("\n\n")
		}
	}

	// Protocols
	b.WriteString(SignalProtocol())
	b.WriteString("\n\n")
	b.WriteString(ConsultProtocol())
	b.WriteString("\n")

	return b.String(), nil
}
