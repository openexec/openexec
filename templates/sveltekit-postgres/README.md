# SvelteKit + Postgres template

An opinionated SvelteKit + Drizzle + Postgres + Lucia stack, scaffolded by
`openexec init --template sveltekit-postgres`. This template is the substrate
on which OpenExec's contract tests, no-stubs gate, and bot users operate.

## Quick start

```bash
cp .env.example .env
docker compose up -d
npm install
npm run db:generate   # after editing a schema
npm run db:migrate
npm run dev
```

`/api/health` should return `{ "status": "ok" }` with HTTP 200.

## Stack opinions (load-bearing)

These conventions exist so OpenExec's later blocks (contract generation,
no-stubs gate, bot users) can reason about this codebase. Do not drift from
them without updating the corresponding OpenExec blueprints.

1. **Mutations go through `+server.ts` (REST), never SvelteKit form actions.**
   Every write path is a JSON REST handler that returns a `Response`. This
   makes every mutation reachable from contract tests and bot users with a
   single HTTP client.

2. **Every mutating handler reads session via `event.locals.user`.** The
   session is populated in `src/hooks.server.ts`. Handlers must never touch
   cookies directly beyond what Lucia provides.

3. **Drizzle schemas live in `src/lib/db/schema/<entity>.ts`** and are
   re-exported from `src/lib/db/schema/index.ts`. One file per entity.

4. **Every entity has `id: uuid` (default random), `created_at`, and
   `updated_at`**, all `timestamp with time zone`, with `defaultNow()` on
   create. Sessions are the only exception (Lucia owns the schema).

5. **Migrations live in `drizzle/`** and are generated with
   `npm run db:generate`. Migrations must be reversible — if you cannot write
   a reverse migration, split the change.

6. **Contract tests live in `tests/contracts/<feature>/<operation>.test.ts`**
   and run against a real Postgres via `testcontainers`. No mocks. See
   `tests/contracts/README.md`.

7. **REST responses go through `src/lib/api/response.ts`** helpers (`ok`,
   `json`, `error`) so response shape stays uniform across the app.

## Layout

```
src/
  app.html                     SvelteKit shell
  app.d.ts                     Locals typing (user, session)
  hooks.server.ts              Lucia session middleware
  lib/
    api/response.ts            REST helpers
    auth/lucia.ts              Lucia setup
    auth/password.ts           scrypt-based password hashing
    db/client.ts               Drizzle client
    db/migrate.ts              Migration runner
    db/schema/                 One file per entity
  routes/
    +layout.svelte
    +page.svelte
    api/health/+server.ts
    api/auth/{signup,login,logout}/+server.ts
tests/
  setup.ts                     testcontainers Postgres lifecycle
  helpers.ts                   createTestUser, apiCall, resetDb
  contracts/                   Generated contract tests
drizzle/                       Generated migrations
docker-compose.yml             Local Postgres 16
.openexec/config.json          quality gates pre-wired
```

## Scripts

| Script                | What it does                                     |
| --------------------- | ------------------------------------------------ |
| `npm run dev`         | Start SvelteKit dev server                       |
| `npm run build`       | Build production bundle (adapter-node)           |
| `npm run db:generate` | Generate Drizzle migrations from schema          |
| `npm run db:migrate`  | Apply migrations to the DB in `DATABASE_URL`     |
| `npm test`            | Run unit tests                                   |
| `npm run test:contract` | Run contract tests against a testcontainer DB |
| `npm run test:e2e`    | Run Playwright E2E tests                         |
| `npm run lint`        | ESLint                                           |
| `npm run format`      | Prettier write                                   |
