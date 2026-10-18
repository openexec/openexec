// Live browser check of the real application; deterministic network fixtures
// avoid provider execution, credentials, and production state changes.
import { createServer } from 'vite'
import { chromium, expect } from '@playwright/test'
import { fileURLToPath } from 'node:url'
const root = fileURLToPath(new URL('..', import.meta.url))
const server = await createServer({ root, server: { host: '127.0.0.1', port: 0, hmr: false } })
let browser
try {
  await server.listen()
  const address = server.httpServer.address()
  browser = await chromium.launch({ headless: true })
  const page = await browser.newPage()
  const errors = []
  page.on('pageerror', error => errors.push(error.message))
  await page.route('**/api/**', route => route.fulfill({ json:
    route.request().url().includes('approvals') ? { approvals: [] } : [] }))
  let socket
  await page.routeWebSocket('**/ws', ws => { socket = ws })
  await page.goto(`http://127.0.0.1:${address.port}`)
  await page.getByRole('button', { name: 'Open panel', exact: true }).click()
  await expect.poll(() => Boolean(socket)).toBe(true)
  let sequence = 0
  const send = (type, name, extra = {}) => socket.send(JSON.stringify({ type: 'event', payload: {
    id: `d6-${++sequence}`, type, kind: 'stage', timestamp: new Date().toISOString(),
    metadata: { stage_name: name, attempt: 2 }, ...extra,
  } }))
  for (const [name, label] of [['fix_lint', 'Fix Lint'], ['production_readiness_check', 'Production Readiness']]) {
    const stage = page.locator('.stage-timeline__stage').filter({ hasText: label })
    send('stage_start', name)
    await expect(stage).toHaveClass(/stage--running/)
    send('stage_failed', name, { error: 'check refused' })
    await expect(stage).toHaveClass(/stage--failed/)
    await expect(stage.getByTitle('check refused')).toBeVisible()
    send('stage_retry', name)
    await expect(stage.getByText('Attempt 2')).toBeVisible()
    send('stage_start', name)
    await expect(stage).toHaveClass(/stage--running/)
    send('stage_complete', name)
    await expect(stage).toHaveClass(/stage--completed/)
  }
  send('stage_start', 'unknown_stage')
  await page.getByRole('button', { name: 'Close panel', exact: true }).click()
  await page.getByRole('button', { name: 'Open panel', exact: true }).click()
  await expect(page.locator('.stage-timeline__stage--completed')).toHaveCount(2)
  await expect(page.locator('.stage-timeline__stage--running')).toHaveCount(0)
  await page.screenshot({ path: '/tmp/openexec-d6-timeline.png' })
  await page.reload()
  await page.getByRole('button', { name: 'Open panel', exact: true }).click()
  await expect(page.locator('.stage-timeline')).toHaveCount(0)
  expect(errors).toEqual([])
  console.log('D6 browser PASS: WebSocket -> real ChatPage -> timeline; failure, retry, completion, unknown stage, panel reopen, reload reset; no page errors')
} finally {
  await browser?.close()
  await server.close()
}
