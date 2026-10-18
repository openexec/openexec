import { test, expect } from '@playwright/test'

for (const endpoint of ['symbols', 'envs']) {
  test(`Knowledge Hub navigation, ${endpoint} failure, Retry, reopen and reload`, async ({ page }) => {
    let failure = ''
    let empty = false
    const requests: string[] = []
    await page.route('**/api/**', async route => {
      const path = new URL(route.request().url()).pathname
      if (path.startsWith('/api/v1/knowledge/')) requests.push(path)
      const kind = path.split('/').pop()
      const supported = kind === 'symbols' || kind === 'envs'
      const body = empty ? {} : kind === 'symbols'
        ? { symbols: [{ name: 'BrowserExample', kind: 'function', file_path: 'example.go', start_line: 1, end_line: 3, purpose: 'Browser purpose' }] }
        : { environments: [{ env: 'dev', runtime_type: 'local', topology: 'localhost' }] }
      await route.fulfill({ status: !supported ? 404 : kind === failure ? 503 : 200, json: body })
    })
    const open = () => page.getByRole('button', { name: 'Knowledge Hub', exact: true }).click()
    const healthy = async () => {
      await expect(page.getByText('BrowserExample', { exact: true })).toBeVisible()
      await expect(page.getByText('DEV', { exact: true })).toBeVisible()
      await expect(page.getByRole('alert')).toHaveCount(0)
      await expect(page.getByText(/policies|enforces automatically/i)).toHaveCount(0)
    }
    await page.goto('/')
    await open()
    await healthy()
    await page.getByRole('button', { name: 'Usage', exact: true }).click()
    failure = endpoint
    await open()
    await expect(page.getByRole('alert')).toContainText('Some knowledge endpoints failed to load')
    failure = ''
    await page.getByRole('button', { name: 'Retry' }).click()
    await healthy()
    await page.reload()
    await open()
    await healthy()
    await page.getByRole('button', { name: 'Usage', exact: true }).click()
    empty = true
    await open()
    await expect(page.getByText('No symbols indexed.')).toBeVisible()
    await expect(page.getByText('No environments recorded.')).toBeVisible()
    await expect(page.getByRole('alert')).toHaveCount(0)
    // Development StrictMode may repeat mount effects; every request must still be supported.
    expect([...new Set(requests)].sort()).toEqual(['/api/v1/knowledge/envs', '/api/v1/knowledge/symbols'])
    expect(requests.filter(path => path.endsWith('/symbols')).length).toBeGreaterThanOrEqual(5)
    expect(requests.filter(path => path.endsWith('/envs')).length).toBeGreaterThanOrEqual(5)
  })
}
