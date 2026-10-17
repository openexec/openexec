import { test, expect, type WebSocketRoute } from '@playwright/test'

test('D4 default app requests the real approval route and hides an unconfigured gate after reload', async ({ page, request }) => {
  let socket: WebSocketRoute | undefined
  await page.routeWebSocket('**/ws', ws => { socket = ws })
  let configured = true
  // Seed a visible approval, then use real Go handlers for unconfigured responses.
  await page.route('**/api/**', async route => {
    if (route.request().url().includes('/approvals')) {
      if (configured) return route.fulfill({ json: { approvals: [{
        id: 'a1', runId: 'run1', toolName: 'write_file', description: 'D4 pending write',
        riskLevel: 'medium', status: 'pending', createdAt: '2026-10-18T00:00:00Z',
      }] } })
      return route.continue()
    }
    return route.fulfill({ json: [] })
  })
  const paths: string[] = []
  page.on('request', req => {
    if (req.url().includes('/approvals')) paths.push(new URL(req.url()).pathname)
  })
  await page.goto('/')
  await page.getByRole('button', { name: 'Open panel', exact: true }).click()
  await expect(page.getByText('D4 pending write', { exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Approve', exact: true })).toBeVisible()
  configured = false
  const unavailable = page.waitForResponse(res => res.url().includes('/approvals') && res.status() === 503)
  // A stage event starts the hook's real approval polling without executing a tool.
  await expect.poll(() => !!socket).toBe(true)
  socket!.send(JSON.stringify({ type: 'event', payload: {
    type: 'stage_start', metadata: { stage_name: 'implement' },
  } }))
  expect(await (await unavailable).json()).toEqual({ error: 'approval gate not configured' })
  await expect(page.getByText('Pending Approvals', { exact: true })).toHaveCount(0)
  await expect(page.getByText('D4 pending write', { exact: true })).toHaveCount(0)
  await expect(page.locator('.chat-layout__right-panel')).toBeVisible()
  const response = page.waitForResponse(res => res.url().includes('/approvals'))
  await page.reload()
  const approvals = await response
  expect(approvals.status()).toBe(503)
  expect(await approvals.json()).toEqual({ error: 'approval gate not configured' })
  await page.getByRole('button', { name: 'Open panel', exact: true }).click()
  await expect(page.locator('.chat-layout__right-panel')).toBeVisible()
  await expect(page.getByText('Pending Approvals', { exact: true })).toHaveCount(0)
  await expect(page.getByText(/Failed to fetch approvals/)).toHaveCount(0)
  // Development StrictMode can repeat mount effects. Every request must use the correct route.
  expect(paths.length).toBeGreaterThanOrEqual(2)
  expect(new Set(paths)).toEqual(new Set(['/api/v1/approvals']))
  // No approval effects become available without a gate.
  for (const action of ['approve', 'reject']) {
    const res = await request.post(`http://127.0.0.1:18080/api/v1/approvals/a1/${action}`)
    expect(res.status()).toBe(503)
    expect(await res.json()).toEqual({ error: 'approval gate not configured' })
  }
})
