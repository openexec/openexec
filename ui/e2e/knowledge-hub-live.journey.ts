import { test, expect } from '@playwright/test'

test('coordinator-backed Knowledge Hub opens, reopens and reloads persisted data', async ({ page, request }) => {
  // Exercise actual middleware refusal as well as authenticated success.
  for (const endpoint of ['symbols', 'envs']) {
    const denied = await request.get(`/api/v1/knowledge/${endpoint}`, {
      headers: { Authorization: '' },
    })
    expect(denied.status()).toBe(401)
  }
  const paths: string[] = []
  page.on('request', req => {
    const path = new URL(req.url()).pathname
    if (path.startsWith('/api/v1/knowledge/')) paths.push(path)
  })
  const open = async () => {
    const symbols = page.waitForResponse(res => res.url().endsWith('/api/v1/knowledge/symbols'))
    const envs = page.waitForResponse(res => res.url().endsWith('/api/v1/knowledge/envs'))
    await page.getByRole('button', { name: 'Knowledge Hub', exact: true }).click()
    const [sym, env] = await Promise.all([symbols, envs])
    expect(sym.status()).toBe(200)
    expect(env.status()).toBe(200)
    expect((await sym.json()).symbols).toEqual(expect.arrayContaining([expect.objectContaining({ name: 'LiveKnowledgeExample' })]))
    expect((await env.json()).environments).toEqual(expect.arrayContaining([expect.objectContaining({ env: 'dev' })]))
    await expect(page.getByText('LiveKnowledgeExample', { exact: true })).toBeVisible()
    await expect(page.getByText('DEV', { exact: true })).toBeVisible()
    await expect(page.getByRole('alert')).toHaveCount(0)
    await expect(page.getByText(/policies|enforces automatically/i)).toHaveCount(0)
  }
  await page.goto('/')
  await open()
  await page.getByRole('button', { name: 'Usage', exact: true }).click()
  await open()
  await page.reload()
  await open()
  expect(paths.sort()).toEqual([
    ...Array<string>(3).fill('/api/v1/knowledge/envs'),
    ...Array<string>(3).fill('/api/v1/knowledge/symbols'),
  ].sort())
})
