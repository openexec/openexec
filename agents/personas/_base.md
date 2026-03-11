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
</principles>
