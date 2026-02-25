package render

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/prompt/manifest"
	"github.com/openexec/openexec/internal/prompt/persona"
	"github.com/openexec/openexec/internal/prompt/workflow"
)

// expandedWorkflow holds a workflow with params already expanded.
type expandedWorkflow struct {
	ID           string
	Instructions string
	Process      string
}

// RenderAgent loads decomposed agent definitions from agentsDir and produces
// a standalone monolithic markdown+XML string for HITL use.
func RenderAgent(agentsDir, agentName string) (string, error) {
	manifests := manifest.NewStore(filepath.Join(agentsDir, "manifests"))
	personas := persona.NewStore(filepath.Join(agentsDir, "personas"))
	workflows := workflow.NewStore(filepath.Join(agentsDir, "workflows"))

	// Load manifest.
	mf, err := manifests.Get(agentName)
	if err != nil {
		return "", fmt.Errorf("render %s: %w", agentName, err)
	}

	// Load merged persona.
	p, err := personas.Get(mf.Persona)
	if err != nil {
		return "", fmt.Errorf("render %s: %w", agentName, err)
	}

	// Load and expand all workflows.
	var wfs []expandedWorkflow
	for _, wref := range mf.Workflows {
		tmpl, err := workflows.Get(wref.ID)
		if err != nil {
			return "", fmt.Errorf("render %s: %w", agentName, err)
		}

		params := wref.Params
		if err := workflow.Validate(tmpl.Params, params); err != nil {
			return "", fmt.Errorf("render %s: workflow %q: %w", agentName, wref.ID, err)
		}

		instr, err := workflow.Expand(tmpl.Instructions, params)
		if err != nil {
			return "", fmt.Errorf("render %s: workflow %q instructions: %w", agentName, wref.ID, err)
		}
		proc, err := workflow.Expand(tmpl.Process, params)
		if err != nil {
			return "", fmt.Errorf("render %s: workflow %q process: %w", agentName, wref.ID, err)
		}

		wfs = append(wfs, expandedWorkflow{
			ID:           wref.ID,
			Instructions: instr,
			Process:      proc,
		})
	}

	// Build output.
	var b strings.Builder

	// YAML frontmatter.
	fmt.Fprintf(&b, "---\nname: %q\ndescription: %q\n---\n\n", mf.Name, mf.Description)

	// Preamble.
	b.WriteString(preamble)
	b.WriteString("\n\n")

	// XML code block.
	b.WriteString("```xml\n")

	// Agent opening tag.
	displayName := titleCase(mf.Name)
	fmt.Fprintf(&b, "<agent id=%q name=%q title=%q>\n",
		mf.Name+".agent.yaml", displayName, mf.Title)

	// Activation block.
	b.WriteString(activationBlock(mf.Name))

	// Persona block.
	b.WriteString("  <persona>\n")
	fmt.Fprintf(&b, "    <role>%s</role>\n", escapeXML(p.Role))
	fmt.Fprintf(&b, "    <identity>%s</identity>\n", escapeXML(p.Identity))
	fmt.Fprintf(&b, "    <communication_style>%s</communication_style>\n", escapeXML(p.CommunicationStyle))
	fmt.Fprintf(&b, "    <principles>%s</principles>\n", escapeXML(p.Principles))
	b.WriteString("  </persona>\n")

	// Prompts block.
	b.WriteString("  <prompts>\n")
	for _, wf := range wfs {
		fmt.Fprintf(&b, "    <prompt id=%q>\n", wf.ID)
		b.WriteString("      <content>\n")
		fmt.Fprintf(&b, "<instructions>\n%s\n</instructions>\n", wf.Instructions)
		fmt.Fprintf(&b, "<process>\n%s\n</process>\n", wf.Process)
		b.WriteString("      </content>\n")
		b.WriteString("    </prompt>\n")
	}
	b.WriteString("  </prompts>\n")

	// Menu block.
	b.WriteString(menuXML(wfs))

	b.WriteString("</agent>\n")
	b.WriteString("```\n")

	return b.String(), nil
}

const preamble = "You must fully embody this agent's persona and follow all activation instructions exactly as specified. NEVER break character until given an exit command."

// activationBlock returns the static activation block XML with the agent name.
func activationBlock(agentName string) string {
	return fmt.Sprintf(`<activation critical="MANDATORY">
      <step n="1">Load persona from this current agent file (already in context)</step>
      <step n="2">IMMEDIATE ACTION REQUIRED - BEFORE ANY OUTPUT:
          - Load and read {project-root}/_bmad/apexflow:agents:%s/config.yaml NOW
          - Store ALL fields as session variables: {user_name}, {communication_language}, {output_folder}
          - VERIFY: If config not loaded, STOP and report error to user
          - DO NOT PROCEED to step 3 until config is successfully loaded and variables stored
      </step>
      <step n="3">Remember: user's name is {user_name}</step>
      <step n="4">Show greeting using {user_name} from config, communicate in {communication_language}, then display numbered list of ALL menu items from menu section</step>
      <step n="5">STOP and WAIT for user input - do NOT execute menu items automatically - accept number or cmd trigger or fuzzy command match</step>
      <step n="6">On user input: Number -> process menu item[n] | Text -> case-insensitive substring match | Multiple matches -> ask user to clarify | No match -> show "Not recognized"</step>
      <step n="7">When processing a menu item: Check menu-handlers section below - extract any attributes from the selected menu item (workflow, exec, tmpl, data, action, validate-workflow) and follow the corresponding handler instructions</step>
      <menu-handlers>
        <handlers>
          <handler type="action">
            When menu item has: action="#id" -> Find prompt with id="id" in current agent XML, follow its content
            When menu item has: action="text" -> Follow the text directly as an inline instruction
          </handler>
        </handlers>
      </menu-handlers>
      <rules>
        <r>ALWAYS communicate in {communication_language} UNLESS contradicted by communication_style.</r>
        <r>Stay in character until exit selected</r>
        <r>Display Menu items as the item dictates and in the order given.</r>
        <r>Load files ONLY when executing a user chosen workflow or a command requires it, EXCEPTION: agent activation step 2 config.yaml</r>
      </rules>
</activation>
`, agentName)
}

// menuXML generates the menu block from the workflow list.
func menuXML(wfs []expandedWorkflow) string {
	var b strings.Builder
	b.WriteString("  <menu>\n")

	// Standard items at top.
	b.WriteString("    <item cmd=\"MH or fuzzy match on menu or help\">[MH] Redisplay Menu Help</item>\n")
	b.WriteString("    <item cmd=\"CH or fuzzy match on chat\">[CH] Chat with the Agent about anything</item>\n")

	// Workflow items.
	for _, wf := range wfs {
		cmd := cmdAbbrev(wf.ID)
		label := strings.ReplaceAll(wf.ID, "-", " ")
		desc := firstSentence(wf.Instructions)
		fmt.Fprintf(&b, "    <item cmd=%q action=\"#%s\">[%s] %s</item>\n",
			cmd+" or fuzzy match on "+label, wf.ID, cmd, desc)
	}

	// Standard items at bottom.
	b.WriteString("    <item cmd=\"DA or fuzzy match on exit, leave, goodbye or dismiss agent\">[DA] Dismiss Agent</item>\n")

	b.WriteString("  </menu>\n")
	return b.String()
}

// cmdAbbrev generates a short command abbreviation from a workflow ID.
// Single words use first two characters uppercased; hyphenated IDs use
// the first character of each word uppercased.
func cmdAbbrev(id string) string {
	parts := strings.Split(id, "-")
	if len(parts) == 1 {
		if len(id) >= 2 {
			return strings.ToUpper(id[:2])
		}
		return strings.ToUpper(id)
	}
	var abbrev strings.Builder
	for _, part := range parts {
		if len(part) > 0 {
			abbrev.WriteByte(part[0])
		}
	}
	return strings.ToUpper(abbrev.String())
}

// firstSentence returns the first line of text, trimmed.
func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// escapeXML escapes special XML characters in content.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// titleCase converts a lowercase name to title case.
func titleCase(s string) string {
	if len(s) == 0 {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
