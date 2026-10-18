import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import KnowledgeHub from './KnowledgeHub'

const symbols = { symbols: [{ name: 'Example', kind: 'function', file_path: 'example.go', start_line: 1, end_line: 3, purpose: 'Example purpose' }] }
const environments = { environments: [{ env: 'dev', runtime_type: 'local', topology: 'localhost' }] }
const endpoints = ['/api/v1/knowledge/symbols', '/api/v1/knowledge/envs']
afterEach(() => { cleanup(); vi.unstubAllGlobals() })

function mockService(empty = false) {
  let failed = ''
  const fetcher = vi.fn(async (url: string) => ({
    ok: endpoints.includes(url) && url !== failed,
    json: async () => empty ? {} : url.endsWith('symbols') ? symbols : environments,
  }))
  vi.stubGlobal('fetch', fetcher)
  return { fetcher, fail: (url: string) => { failed = url } }
}

it.each([false, true])('renders supported data/empty states without warnings (empty=%s)', async empty => {
  const { fetcher } = mockService(empty)
  render(<KnowledgeHub />)
  expect(screen.getByText('Loading Project Knowledge Map...')).toBeInTheDocument()
  await screen.findByText(empty ? 'No symbols indexed.' : 'Example')
  expect(screen.getByText(empty ? 'No environments recorded.' : 'DEV')).toBeInTheDocument()
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(screen.queryByText(/policies|enforces automatically/i)).not.toBeInTheDocument()
  expect(fetcher.mock.calls.map(([url]) => url)).toEqual(endpoints)
})

it.each(endpoints)('reports %s failure, retries and reopens using only supported endpoints', async endpoint => {
  const service = mockService()
  service.fail(endpoint)
  const first = render(<KnowledgeHub />)
  expect(await screen.findByRole('alert')).toHaveTextContent('Some knowledge endpoints failed to load')
  service.fail('')
  fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
  await waitFor(() => expect(screen.queryByRole('alert')).not.toBeInTheDocument())
  await screen.findByText('Example')
  expect(screen.getByText('DEV')).toBeInTheDocument()
  first.unmount()
  render(<KnowledgeHub />)
  await screen.findByText('Example')
  expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  expect(service.fetcher.mock.calls.map(([url]) => url)).toEqual([...endpoints, ...endpoints, ...endpoints])
})
