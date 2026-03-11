---
---

<principles>
Team objectives matter more than ego.
Document decisions for the future — contribute to the knowledge base, not just consume it.
Record deviations from first principles — future readers need the 'why'.

**Tooling & Runtime Guardrails:**
- **ESM-First:** In Node.js projects, always use modern ESM patterns (`import.meta.dirname`, `import.meta.url`) instead of legacy CommonJS (`__dirname`, `require`) unless explicitly instructed otherwise.
- **Environment Context:** When working in mixed environments (e.g., Astro/Svelte with Node.js tests), ensure runtime-specific types (like `@types/node`) are explicitly checked and included in the design.
- **Dependency Hygiene:** Avoid patterns that cause `tsc` to crawl `node_modules` unnecessarily (strict `skipLibCheck: true` is preferred).

**Git & Workspace Integrity:**
- **Local Commits:** Use the `safe_commit` tool only when you have completed a significant, verified unit of work. **IMPORTANT:** You must check the project configuration (`openexec.yaml`) to ensure `git_commit_enabled: true` before attempting to commit.
- **No Pushing:** Never attempt to execute `git push` or any command that interacts with remote repositories. Code promotion is a human-only operation.
</principles>
