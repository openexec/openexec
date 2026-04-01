# Skills Quick Start Guide

Get started with OpenExec Skills in 5 minutes.

## 1. Import Existing Claude Skills (30 seconds)

```bash
# Import all your Claude Code skills
openexec skills import --from-claude

# Verify they were imported
openexec skills list
```

## 2. Use Skills Automatically (Zero config)

```bash
# Skills are auto-selected by BitNet based on your query
openexec run --task "Create a glassmorphism card component"
# → BitNet selects 'ui-ux-pro-max' skill automatically

openexec run --task "Design microservices architecture"
# → BitNet selects 'system-design' skill automatically
```

## 3. Search Within Skills

```bash
# Search across all skills
openexec skills search "color palette"

# Search within specific skill
openexec skills search --skill ui-ux-pro-max "glassmorphism"
```

## 4. Create Your First Skill (2 minutes)

```bash
# Create skill directory
mkdir -p ~/.openexec/skills/my-team-conventions

# Create SKILL.md
cat > ~/.openexec/skills/my-team-conventions/SKILL.md << 'EOF'
---
name: my-team-conventions
description: "My team's coding conventions and best practices"
categories: [custom]
tags: [conventions, style-guide]
when_to_use: "When writing or reviewing code for my team"
priority: high
---

# My Team Conventions

## Naming
- Use camelCase for variables
- Use PascalCase for types/classes
- Use UPPER_SNAKE_CASE for constants

## Error Handling
- Always check errors
- Use custom error types
- Log errors with context

## Testing
- Write tests for all public functions
- Use table-driven tests
- Aim for 80%+ coverage
EOF

# Use your new skill
openexec run --task "Refactor this code to follow team conventions"
```

## 5. Skill with Search (5 minutes)

```bash
# Create skill with searchable database
mkdir -p ~/.openexec/skills/my-api-patterns/{data,scripts}

# Create database
cat > ~/.openexec/skills/my-api-patterns/data/patterns.csv << 'EOF'
pattern,description,example
REST,Standard REST API design,"GET /users/{id}"
GraphQL,Query-based API,"query { user(id: 1) { name } }"
gRPC,High-performance RPC,"service UserService { rpc GetUser... }"
EOF

# Create search script
cat > ~/.openexec/skills/my-api-patterns/scripts/search.py << 'EOF'
#!/usr/bin/env python3
import sys
import csv

query = sys.argv[1].lower() if len(sys.argv) > 1 else ""

with open('../data/patterns.csv') as f:
    reader = csv.DictReader(f)
    for row in reader:
        if query in row['pattern'].lower() or query in row['description'].lower():
            print(f"{row['pattern']}: {row['description']}")
            print(f"  Example: {row['example']}\n")
EOF
chmod +x ~/.openexec/skills/my-api-patterns/scripts/search.py

# Create SKILL.md
cat > ~/.openexec/skills/my-api-patterns/SKILL.md << 'EOF'
---
name: my-api-patterns
description: "API design patterns and examples"
categories: [backend]
tags: [api, rest, graphql, grpc]
when_to_use: "When designing or reviewing APIs"
has_search_engine: true
search_command: "python3 scripts/search.py"
---

# API Patterns

Search the database for API design patterns.
EOF

# Test it
openexec skills search --skill my-api-patterns "REST"
```

## Common Commands

```bash
# List all skills
openexec skills list

# List skills by category
openexec skills list --category design

# Get skill details
openexec skills info ui-ux-pro-max

# Enable/disable skill
openexec skills enable my-skill
openexec skills disable my-skill

# Update imported skills
openexec skills import --from-claude --update

# Remove skill
openexec skills remove my-skill
```

## Tips

1. **Skills are auto-selected** - You don't need to manually specify skills
2. **BitNet does the work** - It picks relevant skills based on your query
3. **More skills = better** - Unlike Claude Code, OpenExec scales to hundreds of skills
4. **Use categories and tags** - They help with routing and discovery
5. **Add when_to_use** - Helps BitNet understand when to select your skill

## Troubleshooting

**Skill not being selected?**
- Check `when_to_use` description is clear
- Add relevant tags
- Set priority to `high` for critical skills

**Skill search not working?**
- Verify `search_command` is executable
- Test script manually: `cd ~/.openexec/skills/<skill> && python3 scripts/search.py "test"`

**Import failed?**
- Check Claude skills exist: `ls ~/.claude/skills/`
- Try specific path: `openexec skills import --path /path/to/skills`

## Next Steps

- Read the [full Skills System documentation](./SKILLS_SYSTEM.md)
- Check out [example skills](https://github.com/openexec/skills-examples)
- Share your skills with the community
