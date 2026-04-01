# OpenExec Skills System

**Version:** 1.0  
**Status:** Design Document  
**Date:** 2026-04-01

---

## Overview

The OpenExec Skills System provides **Claude Code-compatible skill support** with intelligent routing via BitNet/DCP. Skills are modular knowledge packages that provide domain expertise (UI/UX design, architecture patterns, testing strategies, etc.) to the AI assistant.

### Key Features

- **Claude Skill Compatibility** - Import and use existing Claude Code skills
- **Intelligent Routing** - BitNet/DCP selects only relevant skills for each query
- **Internal Search** - Skills can include BM25 or other search engines
- **Category/Tag System** - Organize skills for easy discovery
- **Multi-Source Loading** - Built-in, user, project, and imported skills

---

## Skill Format

Skills use the same format as Claude Code, with OpenExec extensions:

```yaml
# .openexec/skills/ui-ux-pro-max/SKILL.md
---
name: ui-ux-pro-max
description: "UI/UX design intelligence for web and mobile"
categories: [design, frontend, ui, ux]
tags: [react, vue, colors, typography, accessibility]
when_to_use: "When designing UI components, choosing colors, or reviewing UX"
priority: high
has_search_engine: true
search_command: "python3 scripts/search.py"
---

# Skill Content

## Searchable Databases

This skill includes searchable databases:
- `data/styles.csv` - 67 UI styles
- `data/colors.csv` - 161 color palettes  
- `data/typography.csv` - 57 font pairings

### Query Examples
```bash
# Search by domain
python3 scripts/search.py "glassmorphism" --domain style

# Search colors
python3 scripts/search.py "blue palette" --domain color
```
```

### Frontmatter Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | ✅ | Unique skill identifier |
| `description` | ✅ | Short description of what the skill provides |
| `categories` | ❌ | High-level categories (design, backend, etc.) |
| `tags` | ❌ | Specific technologies/topics (react, aws, etc.) |
| `when_to_use` | ❌ | Natural language guidance for routing |
| `priority` | ❌ | `high`, `medium`, or `low` (affects routing) |
| `has_search_engine` | ❌ | Whether skill includes internal search |
| `search_command` | ❌ | Command to execute skill search |

---

## How It Works

### 1. Skill Discovery

```
Query: "Create a glassmorphism card component"
                    ↓
┌─────────────────────────────────────────┐
│  BitNet/DCP Router                      │
│  ├─ Parses intent: "glassmorphism"      │
│  ├─ Detects: UI/design task             │
│  └─ Selects: ui-ux-pro-max skill        │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│  Skill Search (if available)            │
│  ├─ Query: "glassmorphism"              │
│  ├─ Domain: "style"                     │
│  └─ Returns: matching styles/palettes   │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│  Prompt Assembly                        │
│  ├─ System prompt                       │
│  ├─ Available tools                     │
│  ├─ Skill: ui-ux-pro-max (selected)     │
│  ├─ Search results (glassmorphism info) │
│  └─ User query                          │
└─────────────────────────────────────────┘
                    ↓
              LLM Response
```

### 2. Skill Selection vs Claude Code

| Aspect | Claude Code | OpenExec |
|--------|-------------|----------|
| **Loading** | All skills loaded | Only relevant skills selected |
| **Selection** | LLM chooses from all | BitNet pre-selects relevant |
| **Context Usage** | Wastes tokens on irrelevant skills | Only relevant skills in context |
| **Scalability** | Breaks at ~100 skills | Scales to unlimited skills |
| **Speed** | Slower (more tokens) | Faster (pre-filtered) |

---

## CLI Commands

### List Available Skills

```bash
openexec skills list

## Output:
Available Skills:

## Design
  • ui-ux-pro-max - UI/UX design intelligence with 67 styles, 161 palettes
    Tags: react, vue, colors, typography, css
  • brand-guidelines - Brand identity and logo design
    Tags: branding, logos, marketing

## Architecture
  • system-design - Distributed systems design patterns
    Tags: microservices, scalability, patterns
  • clean-architecture - Clean code architecture principles
    Tags: golang, patterns, testing

## Backend
  • api-design - REST/GraphQL API design best practices
    Tags: rest, graphql, openapi
```

### Search Within Skills

```bash
openexec skills search "color palette"

## Output:
Search results for 'color palette':

• ui-ux-pro-max (design)
  Color palette database with 161 palettes including Material Design, Tailwind, etc.

• brand-guidelines (design)
  Brand color selection and palette creation guidelines
```

### Import Claude Skills

```bash
# Import all Claude Code skills
openexec skills import --from-claude

## Output:
Imported 5 skills from ~/.claude/skills
  ✓ ui-ux-pro-max
  ✓ architecture-patterns
  ✓ testing-strategies
  ✓ api-design
  ✓ security-checklist
```

### Create New Skill

```bash
# Scaffold a new skill
openexec skills create my-skill

## Creates:
## ~/.openexec/skills/my-skill/
## ├── SKILL.md
## ├── data/
## └── scripts/
```

---

## Skill Categories

Skills are organized by category for routing and discovery:

| Category | Description | Example Skills |
|----------|-------------|----------------|
| `design` | UI/UX, visual design | ui-ux-pro-max, brand-guidelines |
| `architecture` | System design, patterns | system-design, clean-architecture |
| `backend` | API, database, services | api-design, database-patterns |
| `frontend` | Web UI frameworks | react-patterns, vue-best-practices |
| `devops` | CI/CD, infrastructure | kubernetes-patterns, terraform-guide |
| `security` | Security best practices | security-checklist, threat-modeling |
| `testing` | Testing strategies | unit-testing, e2e-testing |
| `mobile` | iOS, Android development | swiftui-patterns, jetpack-compose |

---

## Directory Structure

```
~/.openexec/
├── skills/
│   ├── builtin/              # Shipped with OpenExec
│   │   ├── code-review/
│   │   ├── architecture-design/
│   │   └── testing-patterns/
│   ├── user/                 # User-created skills
│   │   └── my-custom-skill/
│   └── imported/             # Imported from Claude
│       └── ui-ux-pro-max/
│           ├── SKILL.md
│           ├── data/
│           │   ├── styles.csv
│           │   ├── colors.csv
│           │   └── typography.csv
│           └── scripts/
│               └── search.py
└── config.json

./my-project/
└── .openexec/
    └── skills/               # Project-local skills
        └── project-conventions/
```

---

## Importing from Claude Code

OpenExec is fully compatible with Claude Code skills. To import:

```bash
# 1. Import existing Claude skills
openexec skills import --from-claude

# 2. Or import from specific path
openexec skills import --path /path/to/claude/skills

# 3. Verify import
openexec skills list

# 4. Use in project
openexec run --task "Design a landing page"
# BitNet auto-selects ui-ux-pro-max skill
```

### Claude Skill Compatibility

| Feature | Claude | OpenExec | Notes |
|---------|--------|----------|-------|
| `SKILL.md` format | ✅ | ✅ | Fully compatible |
| Frontmatter | ✅ | ✅ | All fields supported |
| `when_to_use` | ✅ | ✅ | Used for routing |
| Categories | ❌ | ✅ | OpenExec extension |
| Tags | ❌ | ✅ | OpenExec extension |
| Internal search | ✅ | ✅ | BM25, etc. |
| Priority | ❌ | ✅ | OpenExec extension |

---

## Creating Custom Skills

### Basic Skill

```bash
mkdir -p ~/.openexec/skills/my-skill
cat > ~/.openexec/skills/my-skill/SKILL.md << 'EOF'
---
name: my-skill
description: "My custom skill for specific tasks"
categories: [custom]
tags: [mytag, example]
when_to_use: "When working with specific technology or pattern"
priority: medium
---

# My Skill

## Guidelines

1. Step one
2. Step two
3. Step three

## Examples

```python
# Example code
```
EOF
```

### Skill with Search Engine

```bash
mkdir -p ~/.openexec/skills/my-database-skill/{data,scripts}

# Create data
cat > ~/.openexec/skills/my-database-skill/data/items.csv << 'EOF'
id,name,category,description
1,Item One,categoryA,Description of item one
2,Item Two,categoryB,Description of item two
EOF

# Create search script
cat > ~/.openexec/skills/my-database-skill/scripts/search.py << 'EOF'
#!/usr/bin/env python3
import sys
import csv

query = sys.argv[1].lower()

with open('../data/items.csv') as f:
    reader = csv.DictReader(f)
    for row in reader:
        if query in row['name'].lower() or query in row['description'].lower():
            print(f"{row['name']}: {row['description']}")
EOF
chmod +x ~/.openexec/skills/my-database-skill/scripts/search.py

# Create SKILL.md
cat > ~/.openexec/skills/my-database-skill/SKILL.md << 'EOF'
---
name: my-database-skill
description: "Searchable database of items"
categories: [custom]
has_search_engine: true
search_command: "python3 scripts/search.py"
---

# My Database Skill

Search the database using the search script.
EOF
```

---

## Architecture

### Components

```
┌─────────────────────────────────────────┐
│  Skill Registry                         │
│  ├─ Load from multiple sources          │
│  ├─ Index by category/tag               │
│  └─ Manage skill lifecycle              │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│  BitNet/DCP Router                      │
│  ├─ Parse query intent                  │
│  ├─ Match against skill metadata        │
│  ├─ Score relevance                     │
│  └─ Select top-N skills                 │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│  Skill Search (optional)                │
│  ├─ Execute skill's search command      │
│  ├─ Return relevant content             │
│  └─ Include in prompt                   │
└─────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────┐
│  Prompt Builder                         │
│  ├─ System prompt                       │
│  ├─ Tools                               │
│  ├─ Selected skills                     │
│  ├─ Skill search results                │
│  └─ User query                          │
└─────────────────────────────────────────┘
```

### Skill Selection Algorithm

```go
func SelectSkills(query string, context Context) []*Skill {
    // 1. Parse intent with BitNet
    intent := bitnet.Parse(query)
    
    // 2. Find candidate skills
    candidates := registry.FindByCategories(intent.Categories)
    
    // 3. Score each skill
    for _, skill := range candidates {
        score := 0.0
        
        // Category match
        if matchesCategory(skill, intent) {
            score += 10.0
        }
        
        // Tag match
        score += float64(countMatchingTags(skill, intent)) * 5.0
        
        // Keyword match in when_to_use
        if matchesKeywords(skill.WhenToUse, intent.Keywords) {
            score += 3.0
        }
        
        // Priority multiplier
        score *= priorityMultiplier(skill.Priority)
        
        skill.Score = score
    }
    
    // 4. Sort and select top-N within budget
    sortByScore(candidates)
    return selectWithinBudget(candidates, maxTokens)
}
```

---

## Best Practices

### 1. Skill Design

- **Keep skills focused** - One domain per skill
- **Use descriptive when_to_use** - Helps BitNet route correctly
- **Add relevant tags** - Improves searchability
- **Include examples** - Helps LLM understand usage

### 2. Performance

- **Use internal search** - For large datasets, use BM25/vector search
- **Limit skill size** - Keep SKILL.md under ~10KB
- **Set appropriate priority** - High for critical skills, low for optional

### 3. Organization

- **Use categories** - Group related skills
- **Tag consistently** - Use standard tags (react, go, aws, etc.)
- **Version control** - Keep skills in git for collaboration

---

## Examples

### UI/UX Design Task

```bash
openexec run --task "Create a glassmorphism login form"
```

**Skill Selection:**
- BitNet detects: "glassmorphism" → design/UI keyword
- Selected skill: `ui-ux-pro-max`
- Search: "glassmorphism" → returns CSS examples, color palettes
- LLM sees: Tools + ui-ux-pro-max skill + glassmorphism examples

### Architecture Design Task

```bash
openexec run --task "Design microservices architecture for e-commerce"
```

**Skill Selection:**
- BitNet detects: "microservices", "architecture", "e-commerce"
- Selected skills: `system-design`, `clean-architecture`
- LLM sees: Tools + architecture patterns + e-commerce considerations

---

## Migration from Claude Code

### Step-by-Step

```bash
# 1. Check existing Claude skills
ls ~/.claude/skills/

# 2. Import to OpenExec
openexec skills import --from-claude

# 3. Verify import
openexec skills list

# 4. Test with a query
openexec run --task "Design a React component"

# 5. Add OpenExec-specific metadata (optional)
# Edit ~/.openexec/skills/imported/<skill>/SKILL.md
# Add categories, tags, priority
```

---

## Future Enhancements

- [ ] **Skill Marketplace** - Share and discover community skills
- [ ] **Skill Versioning** - Version control for skills
- [ ] **Dynamic Skill Loading** - Load skills on-demand from remote
- [ ] **Skill Composition** - Combine multiple skills
- [ ] **Skill Testing** - Validate skills work correctly

---

## References

- [Claude Code Skills Documentation](https://docs.anthropic.com/claude/docs/skills)
- [UI/UX Pro Max Skill](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill) - Example skill with BM25 search
- [OpenExec Architecture](./ARCHITECTURE.md)
- [BitNet Router](./BITNET_ROUTER.md)
