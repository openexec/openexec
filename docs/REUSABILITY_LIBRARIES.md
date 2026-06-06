# The Reusability Ecosystem: Three-Tier AI Libraries

OpenExec and its companion tools do not let AI agents guess, drift, or write repetitive code from scratch. The ecosystem operates on a strict **Three-Tier Library Hierarchy** that standardizes code generation at every level of abstraction:

```
┌────────────────────────────────────────────────────────┐
│  Tier 1: Architectural Library (blueprints)           │
│  • Folder structures, data models, operation contracts  │
└───────────────────────────┬────────────────────────────┘
                            ▼ Injects structural shape
┌────────────────────────────────────────────────────────┐
│  Tier 2: Functional Library (intent-compiler/packs)    │
│  • Regulatory compliance, auth logic, security rules   │
└───────────────────────────┬────────────────────────────┘
                            ▼ Injects compliance requirements
┌────────────────────────────────────────────────────────┐
│  Tier 3: Code/Implementation Library (Skills Engine)   │
│  • SKILL.md packages, React UI blocks, local snippet   │
└────────────────────────────────────────────────────────┘
```

---

## Tier 1: The Architectural Library (`blueprints`)

**Location:** `/Users/perttu/projects/blueprints`  
**Purpose:** Defines the structural, database, and API routing standards for common product categories.

When an agent is asked to *"build a marketplace with bidding"* or *"create an MCP server,"* it does not generate an ad-hoc layout. Instead, it references a compiled YAML blueprint from Tier 1:

- **Directory Layouts (`modules`):** Standardizes directory mapping (e.g., `src/services/listings`) based on structural analysis of successful open-source codebases.
- **Data Models (`data_models`):** Specifies field names, types, and database tables to prevent SQL schema drift.
- **Operation Contracts (`feature_completeness_contracts`):** Defines absolute acceptance criteria for specific endpoints. For example, a bidding endpoint contract specifies: *"Must persist bids, must mutate listing.current_high_bid, must authenticate user, must trigger a notification."*

---

## Tier 2: The Functional/Specification Library (`intent-compiler/packs`)

**Location:** `/Users/perttu/projects/intent-compiler/packs`  
**Purpose:** Injects pre-authored regulatory, security, and baseline requirements into the task planning phase.

Instead of writing custom logic for standard flows, OpenExec automatically matches and loads functional **packs** based on trigger keywords or structural conditions:

- **Automatic Matching (`applies_when`):** If a user's prompt mentions login, sign up, or registration, the `auth_standard.yaml` pack is loaded. If it relates to financial reporting, `finnish_invoicing_standard.yaml` is matched.
- **Requirements Injection (`requirements`):** Standardizes explicit requirements (e.g., password hashing using bcrypt, logout session termination, token invalidation).
- **Evidence Verification (`evidence_required`):** Explicitly states what automated test logs or system outputs must be captured to mark a task as legally compliant.

---

## Tier 3: The Code/Implementation Library (Skills Engine)

**Location:** `~/.claude/skills/`, `.openexec/skills/`, or `/Users/perttu/projects/claude-skill`  
**Purpose:** Stores reusable code blocks, UI components, testing utilities, and micro-instructions.

This is the implementation engine's "hot execution context" (fully compatible with Claude Code’s `SKILL.md` format):

- **Context Injection:** When an agent works on a specific file or task type (like React components or testing), the system fetches matching `.md` skill files and appends their contents directly to the LLM system prompt.
- **Component Reusability:** Prevents agents from reinventing common components (e.g., using existing tailwind/shadcn structures, shared Go helper functions, or local mock databases) by providing clear code templates.
- **Local Workspace Customization:** Developers can place custom skills under `.openexec/skills/` to teach agents project-specific rules, APIs, and quirks.

---

## How to Leverage This System

### For Humans:
1. **Define Core Rules:** Keep standard requirements (like security baselines or billing laws) in `intent-compiler/packs/`.
2. **Add Standard Architectures:** Add seeds to the `blueprints` project to standardize how new folders and APIs are modeled.
3. **Commit Custom Code Skills:** Place code snippets and UI patterns in `.openexec/skills/` so agents use them automatically when editing your codebase.

### For Agents Studying This Project:
1. **Always Check the Blueprint:** Before writing API endpoints or databases, look for matching definitions in `/Users/perttu/projects/blueprints`. Adhere strictly to the defined schema and operations.
2. **Scan Active Packs:** Check `intent-compiler/packs/` to ensure your implementation fulfills standard compliance, security, and verification requirements.
3. **Inspect the Skills Registry:** Read the contents of active `.openexec/skills/` to inherit existing utility helper methods, mock objects, and architectural patterns. Do not build custom modules if a matching skill-template exists.
