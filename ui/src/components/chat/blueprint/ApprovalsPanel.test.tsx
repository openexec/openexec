import { act, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ApprovalsPanel from './ApprovalsPanel'
import { useBlueprint } from '../../../hooks/useBlueprint'
import type { Approval } from '../../../types/blueprint'

const approval: Approval = {
  id: 'a1', runId: 'run1', toolName: 'write_file', description: 'Write file',
  riskLevel: 'medium', status: 'pending', createdAt: '2026-10-18T00:00:00Z',
}
const response = (status: number, body: unknown) => new Response(JSON.stringify(body), {
  status, statusText: status === 200 ? 'OK' : 'Service Unavailable',
})
const unavailable = () => response(503, { error: 'approval gate not configured' })
afterEach(() => vi.unstubAllGlobals())

describe('D4 approval endpoints', () => {
  it.each(['/api', 'https://example.test/api'])('uses the API base for hook list and both actions: %s', async (apiUrl) => {
    const fetch = vi.fn().mockImplementation(() => Promise.resolve(response(200, { approvals: [approval] })))
    vi.stubGlobal('fetch', fetch)
    const { result } = renderHook(() => useBlueprint({ apiUrl, authToken: 'token', runId: 'run1' }))
    await waitFor(() => expect(result.current.approvals).toHaveLength(1))
    expect(fetch.mock.calls[0]).toEqual([`${apiUrl}/v1/approvals?run_id=run1&status=pending`, { headers: { Authorization: 'Bearer token' } }])
    await act(() => result.current.approveRequest('a1'))
    expect(result.current.approvals[0].status).toBe('approved')
    await act(() => result.current.rejectRequest('a1', 'No'))
    expect(result.current.approvals[0].status).toBe('rejected')
    for (const [index, action] of [[1, 'approve'], [2, 'reject']] as const) {
      expect(fetch.mock.calls[index][0]).toBe(`${apiUrl}/v1/approvals/a1/${action}`)
      expect(fetch.mock.calls[index][1]).toMatchObject({ method: 'POST', headers: { Authorization: 'Bearer token' } })
    }
  })

  it('clears stale hook approvals on not-configured and recovers on refetch', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(response(200, { approvals: [approval] }))
      .mockResolvedValueOnce(unavailable()).mockResolvedValueOnce(response(200, { approvals: [approval] }))
    vi.stubGlobal('fetch', fetch)
    const { result } = renderHook(() => useBlueprint({ apiUrl: '/api' }))
    await waitFor(() => expect(result.current.approvals).toHaveLength(1))
    await act(() => result.current.fetchApprovals())
    expect(result.current.approvals).toEqual([])
    expect(result.current.approvalsError).toBeUndefined()
    await act(() => result.current.fetchApprovals())
    expect(result.current.approvals).toHaveLength(1)
  })

  it('hides the standalone panel on not-configured, including after remount', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => Promise.resolve(unavailable())))
    const first = render(<ApprovalsPanel apiUrl="/api" />)
    await waitFor(() => expect(first.container).toBeEmptyDOMElement())
    first.unmount()
    const second = render(<ApprovalsPanel apiUrl="/api" />)
    await waitFor(() => expect(second.container).toBeEmptyDOMElement())
  })

  it.each([404, 401, 500, 503])('keeps ordinary HTTP %s failures visible in panel and hook', async (status) => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => Promise.resolve(response(status, { error: 'temporary failure' }))))
    render(<ApprovalsPanel apiUrl="/api" />)
    const { result } = renderHook(() => useBlueprint({ apiUrl: '/api' }))
    expect(await screen.findByText(/Failed to fetch approvals/)).toBeVisible()
    await waitFor(() => expect(result.current.approvalsError).toMatch(/Failed to fetch approvals/))
  })


  it('shows a later network failure after an unavailable gate and recovers when filters change', async () => {
    const fetch = vi.fn().mockResolvedValueOnce(unavailable())
      .mockRejectedValueOnce(new Error('Network offline'))
      .mockResolvedValueOnce(response(200, { approvals: [approval] }))
    vi.stubGlobal('fetch', fetch)
    const view = render(<ApprovalsPanel apiUrl="/api" runId="first" />)
    await waitFor(() => expect(view.container).toBeEmptyDOMElement())
    view.rerender(<ApprovalsPanel apiUrl="/api" runId="second" />)
    expect(await screen.findByText('Network offline')).toBeVisible()
    view.rerender(<ApprovalsPanel apiUrl="/api" runId="third" />)
    expect(await screen.findByText('Write file')).toBeVisible()
  })

  it('does not hide malformed 503 responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockImplementation(() => Promise.resolve(new Response('unavailable', { status: 503, statusText: 'Service Unavailable' }))))
    render(<ApprovalsPanel apiUrl="/api" />)
    const { result } = renderHook(() => useBlueprint({ apiUrl: '/api' }))
    expect(await screen.findByText(/Failed to fetch approvals/)).toBeVisible()
    await waitFor(() => expect(result.current.approvalsError).toMatch(/Failed to fetch approvals/))
  })

  it.each(['approve', 'reject'])('lists and performs standalone %s at the correct URL', async (action) => {
    const fetch = vi.fn().mockImplementation(() => Promise.resolve(response(200, { approvals: [approval] })))
    vi.stubGlobal('fetch', fetch)
    render(<ApprovalsPanel apiUrl="/api" authToken="token" runId="run1" />)
    await screen.findByText('Write file')
    expect(fetch.mock.calls[0][0]).toBe('/api/v1/approvals?run_id=run1&status=pending')
    fireEvent.click(screen.getByRole('button', { name: action === 'approve' ? 'Approve' : 'Reject' }))
    if (action === 'reject') fireEvent.click(screen.getByRole('button', { name: /Confirm/i }))
    await waitFor(() => expect(screen.queryByText('Write file')).not.toBeInTheDocument())
    expect(fetch.mock.calls[1][0]).toBe(`/api/v1/approvals/a1/${action}`)
    expect(fetch.mock.calls[1][1]).toMatchObject({ method: 'POST', headers: { Authorization: 'Bearer token' } })
  })
})
