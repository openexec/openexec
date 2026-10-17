import { test, expect } from '@playwright/test'

test('D4 default app requests the real approval route and hides an unconfigured gate after reload', async ({ page, request }) => {
  // Unrelated data routes need repositories; approvals use the real Go handlers.
  await page.route('**/api/**', async route => {
    if (route.request().url().includes('/approvals')) return route.continue()
    return route.fulfill({ json: [] })
  })
  const paths: string[] = []
  page.on('request', req => {
    if (req.url().includes('/approvals')) paths.push(new URL(req.url()).pathname)
  })
  for (const reload of [false, true]) {
    const response = page.waitForResponse(res => res.url().includes('/approvals'))
    if (reload) await page.reload()
    else await page.goto('/')
    const approvals = await response
    expect(approvals.status()).toBe(503)
    expect(await approvals.json()).toEqual({ error: 'approval gate not configured' })
    await expect(page.locator('#root')).not.toBeEmpty()
    await expect(page.getByText('Pending Approvals', { exact: true })).toHaveCount(0)
    await expect(page.getByText(/Failed to fetch approvals/)).toHaveCount(0)
  }
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
