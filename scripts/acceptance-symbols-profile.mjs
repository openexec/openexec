// Acceptance test for the symbols-only MCP profile
// (`openexec mcp-serve --profile symbols`).
//
// Proves the positives — lookup, relations and bounded source read — and, more
// importantly, the negatives: backlog writes, skill proposals, patches, shell
// execution and synchronous stale-graph scanning must all be unavailable, even
// when the ambient environment asks for danger-full-access.
//
// It drives the real server over stdio with exactly the argv Agent Console's
// symbolServerFor() generates, then checks that Codex accepts the same argv as
// per-run `-c mcp_servers.*` overrides.
//
//   go build -o /tmp/openexec-bin ./cmd/openexec
//   node scripts/acceptance-symbols-profile.mjs [openexec-bin] [codex-bin]
//
// Exits non-zero if any check fails.
import { spawn, execFileSync } from 'node:child_process'

const BIN = process.argv[2] || '/tmp/openexec-bin'
const CODEX = process.argv[3] || '/snap/bin/codex'
import { mkdirSync, writeFileSync, rmSync, appendFileSync, existsSync, readdirSync, readFileSync, statSync } from 'node:fs'
import { createHash } from 'node:crypto'

// digestDir hashes every file under a directory, so "unchanged" means the
// bytes, not just the file list.
function digestDir(root) {
  const hash = createHash('sha256')
  const walk = path => {
    for (const entry of readdirSync(path, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      const full = path + '/' + entry.name
      if (entry.isDirectory()) { hash.update('D' + entry.name); walk(full); continue }
      hash.update('F' + entry.name + statSync(full).size)
      hash.update(readFileSync(full))
    }
  }
  if (existsSync(root)) walk(root)
  return hash.digest('hex')
}

const dir = process.env.ACCEPT_DIR || '/tmp/acceptsym'
rmSync(dir, { recursive: true, force: true })
mkdirSync(dir + '/.openexec', { recursive: true })
writeFileSync(dir + '/go.mod', 'module acceptsym\n\ngo 1.25\n')
writeFileSync(dir + '/main.go', `package main

func Helper(n int) int { return n * 2 }

func Caller() int { return Helper(21) }

func main() { _ = Caller() }
`)
const env = { ...process.env, PATH: process.env.PATH + ':/usr/local/go/bin' }
execFileSync('git', ['init', '-q'], { cwd: dir })
execFileSync('git', ['add', '-A'], { cwd: dir })
execFileSync('git', ['-c', 'user.email=a@b.c', '-c', 'user.name=t', 'commit', '-qm', 'i'], { cwd: dir })
execFileSync(BIN, ['knowledge', 'graph', 'scan', '-d', dir], { cwd: dir, env, stdio: 'ignore' })

// Exactly the argv agent-console's symbolServerFor() generates.
const ARGS = ['mcp-serve', '--profile', 'symbols', '--mode', 'suggest', '--workspace', dir]

function client() {
  const proc = spawn(BIN, ARGS, {
    cwd: '/', env: { ...env, OPENEXEC_MODE: 'danger-full-access', WORKSPACE_ROOT: '/' },
  })
  let buf = ''
  const waiters = new Map()
  proc.stdout.on('data', c => {
    buf += c
    let i
    while ((i = buf.indexOf('\n')) >= 0) {
      const line = buf.slice(0, i).trim(); buf = buf.slice(i + 1)
      if (!line.startsWith('{')) continue
      let m; try { m = JSON.parse(line) } catch { continue }
      const w = waiters.get(m.id); if (w) { waiters.delete(m.id); w(m) }
    }
  })
  let id = 1
  const call = (method, params) => new Promise(r => {
    const n = id++; waiters.set(n, r)
    proc.stdin.write(JSON.stringify({ jsonrpc: '2.0', id: n, method, params }) + '\n')
  })
  return { proc, call }
}

const text = r => r?.result?.content?.[0]?.text ?? JSON.stringify(r?.result ?? r?.error)
const results = []
const check = (name, pass, detail = '') => {
  results.push({ name, pass, detail })
  console.log(`${pass ? 'PASS' : 'FAIL'}  ${name}${detail ? ' — ' + detail : ''}`)
}

let { proc, call } = client()
await call('initialize', { protocolVersion: '2024-11-05', capabilities: {}, clientInfo: { name: 'accept', version: '0' } })

const names = ((await call('tools/list', {})).result?.tools ?? []).map(t => t.name)
check('tools/list exposes exactly the 3 symbol tools', names.length === 3 && names.every(n => n.startsWith('symbol_')), names.join(',') || '(none)')

const found = await call('tools/call', { name: 'symbol_find', arguments: { query: 'Helper' } })
const pointer = found.result?.result?.pointers?.[0]
check('symbol_find returns a pointer', !!pointer && pointer.file === 'main.go', pointer ? `${pointer.file}:${pointer.start_line}` : text(found))

const rel = await call('tools/call', { name: 'symbol_relations', arguments: { symbol_id: pointer?.symbol_id } })
check('symbol_relations finds the caller', text(rel).includes('main.Caller'))

const read = await call('tools/call', { name: 'symbol_read', arguments: { symbol_id: pointer?.symbol_id } })
check('symbol_read returns source', text(read).includes('func Helper'))

// Read-only in fact, not only in the description: a lookup against a current
// graph must leave the orchestrator state byte-identical.
const before = digestDir(dir + '/.openexec')
await call('tools/call', { name: 'symbol_find', arguments: { query: 'Caller' } })
await call('tools/call', { name: 'symbol_read', arguments: { symbol_id: pointer?.symbol_id } })
await call('tools/call', { name: 'symbol_relations', arguments: { symbol_id: pointer?.symbol_id } })
const after = digestDir(dir + '/.openexec')
check('lookups leave .openexec unchanged', before === after,
  before === after ? '' : 'orchestrator state was modified by a read')

// Negatives — each must be refused even though the ambient env asks for danger mode.
for (const [name, args] of [
  ['backlog_add_task', { title: 'x', story_id: 'US-MAINT' }],
  ['backlog_claim_story', { story_id: 'US-001' }],
  ['skill_propose', { skill_name: 'x', content: 'y' }],
  ['git_apply_patch', { patch: 'diff --git a/x b/x\n', check_only: false }],
  ['run_shell_command', { command: 'echo pwned' }],
  ['write_file', { path: 'x', content: 'y' }],
  ['memory_read', {}],
  ['read_file', { path: 'main.go' }],
]) {
  const r = await call('tools/call', { name, arguments: args })
  check(`${name} refused`, r.result?.isError === true, text(r).slice(0, 70))
}
proc.stdin.end(); proc.kill()

// Stale graph must refuse, not scan synchronously.
appendFileSync(dir + '/main.go', '\nfunc Added() int { return Helper(1) }\n')
;({ proc, call } = client())
await call('initialize', { protocolVersion: '2024-11-05', capabilities: {}, clientInfo: { name: 'accept', version: '0' } })
const started = Date.now()
const stale = await call('tools/call', { name: 'symbol_find', arguments: { query: 'Helper' } })
const elapsed = Date.now() - started
check('drifted graph refuses instead of scanning',
  stale.result?.isError === true && /stale|does not rebuild/i.test(text(stale)),
  `${elapsed}ms — ${text(stale).slice(0, 80)}`)
proc.stdin.end(); proc.kill()

// Codex, the provider installed on the deployment host, must actually accept
// the -c overrides agent-console generates.
const quoted = ARGS.map(a => JSON.stringify(a)).join(',')
// A CODEX_HOME this script creates itself: pointing at the operator's real one
// reads their config (and fails outright under snap confinement), and reusing
// a directory some earlier run left behind makes the check unrepeatable.
const codexHome = dir + '-codexhome'
rmSync(codexHome, { recursive: true, force: true })
mkdirSync(codexHome, { recursive: true })
try {
  const listed = execFileSync(CODEX, ['mcp', 'list',
    '-c', `mcp_servers.openexec.command=${JSON.stringify(BIN)}`,
    '-c', `mcp_servers.openexec.args=[${quoted}]`,
  ], { encoding: 'utf8', env: { ...env, CODEX_HOME: codexHome } })
  check('codex accepts the generated -c overrides', /openexec/.test(listed) && /--profile symbols/.test(listed),
    listed.split('\n')[1]?.slice(0, 90))
} catch (e) {
  check('codex accepts the generated -c overrides', false,
    ((e.stderr || '') + (e.message || '')).slice(0, 120))
}

// An initialized-but-never-scanned checkout must get no symbol tools, and the
// server must not create a graph database just by being asked.
const bare = dir + '-bare'
rmSync(bare, { recursive: true, force: true })
mkdirSync(bare + '/.openexec', { recursive: true })
writeFileSync(bare + '/main.go', 'package main\n\nfunc main() {}\n')
{
  const p = spawn(BIN, ['mcp-serve', '--profile', 'symbols', '--mode', 'suggest', '--workspace', bare], { cwd: '/', env })
  let b = ''
  const w = new Map()
  p.stdout.on('data', c => {
    b += c
    let i
    while ((i = b.indexOf('\n')) >= 0) {
      const line = b.slice(0, i).trim(); b = b.slice(i + 1)
      if (!line.startsWith('{')) continue
      let m; try { m = JSON.parse(line) } catch { continue }
      const f = w.get(m.id); if (f) { w.delete(m.id); f(m) }
    }
  })
  let n = 1
  const c2 = (method, params) => new Promise(r => { const id = n++; w.set(id, r); p.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n') })
  await c2('initialize', { protocolVersion: '2024-11-05', capabilities: {}, clientInfo: { name: 'accept', version: '0' } })
  const listed = ((await c2('tools/list', {})).result?.tools ?? []).length
  await c2('tools/call', { name: 'symbol_find', arguments: { query: 'main' } })
  p.stdin.end(); p.kill()
  check('unscanned checkout advertises no symbol tools', listed === 0, `${listed} advertised`)
  check('unscanned checkout gets no database created',
    !existsSync(bare + '/.openexec/openexec.db'))
}

// A valid database holding zero graph generations — the state another OpenExec
// feature leaves behind. A file-existence gate passes this and then answers
// "no graph" to every lookup; only a generation check rejects it.
try { execFileSync(BIN, ['knowledge', 'graph', 'symbol', 'Nothing', '-d', bare], { stdio: 'ignore', env }) } catch { /* expected: no graph */ }
{
  const hasDB = existsSync(bare + '/.openexec/openexec.db')
  const p = spawn(BIN, ['mcp-serve', '--profile', 'symbols', '--mode', 'suggest', '--workspace', bare], { cwd: '/', env })
  let b = ''
  const w = new Map()
  p.stdout.on('data', c => {
    b += c
    let i
    while ((i = b.indexOf('\n')) >= 0) {
      const line = b.slice(0, i).trim(); b = b.slice(i + 1)
      if (!line.startsWith('{')) continue
      let m; try { m = JSON.parse(line) } catch { continue }
      const f = w.get(m.id); if (f) { w.delete(m.id); f(m) }
    }
  })
  let n = 1
  const c2 = (method, params) => new Promise(r => { const id = n++; w.set(id, r); p.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n') })
  await c2('initialize', { protocolVersion: '2024-11-05', capabilities: {}, clientInfo: { name: 'accept', version: '0' } })
  const listed = ((await c2('tools/list', {})).result?.tools ?? []).length
  p.stdin.end(); p.kill()
  check('fixture has a real database with no generations', hasDB)
  check('database without a graph generation advertises no tools', listed === 0, `${listed} advertised`)
}

const failed = results.filter(r => !r.pass)
console.log(`\n${results.length - failed.length}/${results.length} checks passed`)
process.exit(failed.length ? 1 : 0)
